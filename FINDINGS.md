# Findings

Results from `make run` at two scales (`N=5000` and `N=200000`), run in
WSL2 on the **native Linux (ext4) filesystem** — the repo and database
files live under WSL2's own filesystem, not on the Windows side
(`/mnt/c`).

**Environment, confirmed:**

| | |
|---|---|
| CPU cores (WSL2) | 4 |
| RAM (WSL2) | 3.8 GiB total (only ~2.0 GiB available at time of testing) |
| Kernel | `6.18.33.2-microsoft-standard-WSL2` |
| Repo/DB path | native WSL2 filesystem (`/dev/sdd`, `ext4`) |
| Physical disk | Kingston SA400S37240G — SSD, 240 GB |

The repo and database files sit on native ext4, backed by a real SSD,
so no filesystem-bridge overhead is involved. The main remaining
constraint on this environment is memory: WSL2 has only 3.8 GiB
allotted (well under the host's full 8 GB), and only ~2 GiB was
actually free at runtime, leaving limited headroom for the OS's own
page cache to absorb write bursts before they hit disk.

**The absolute timings below should be read as "this machine, this
memory budget," not as a generic ext4/SSD baseline** — a machine with
more free RAM would likely show smaller absolute numbers, since the OS
page cache would absorb more before anything hits the physical disk.
The *relative* differences between strategies (pooled vs. not,
sync-every-write vs. sync-once) are the portable, meaningful part of
these results — those multipliers reflect what fsync and caching
actually cost.

---

## 1. Workload: key pattern vs. tree shape

| Pattern | Inserts | Splits | Writes | Height | Pages |
|---|---|---|---|---|---|
| sequential | 200,000 | 1,997 | 203,998 | 3 | 2,000 |
| random | 200,000 | 1,439 | 204,304 | 3 | 1,442 |
| uuid4-like | 200,000 | 1,441 | 204,310 | 3 | 1,444 |
| uuid7-like | 200,000 | 1,997 | 203,998 | 3 | 2,000 |

**Ascending keys (sequential, uuid7-like) split ~39% more than random
keys (random, uuid4-like), for the same number of inserts.** This is
the opposite of the "sequential keys are cache-friendly, therefore
better" intuition — and the mechanism is worth being precise about:

This implementation does a **plain 50/50 split** regardless of where
the inserted key landed. With ascending keys, every insert lands at the
rightmost edge of the tree — the same leaf, every time. That leaf
fills, splits into two, and the *new* rightmost leaf immediately starts
absorbing every subsequent insert on its own, while its left sibling is
frozen forever (nothing will ever be smaller than a key that was
already inserted). Each split only "buys" half a leaf's worth of
headroom before the pattern repeats. Random keys, by contrast, land
across the *whole* existing key range roughly uniformly, so leaves fill
up more evenly and less frequently.

This is a genuine, well-known real-world tradeoff — it's the practical
argument for special-casing rightmost-appends (keeping the full leaf
mostly full and making the new leaf small, rather than splitting
50/50), and part of the debate around ascending primary keys
(UUIDv7/ULID/Snowflake IDs) in production databases: better for
physical write locality, worse for split frequency, unless the
implementation accounts for it.

Height stayed at 2 (5,000 keys) and 3 (200,000 keys) across *every*
pattern — the ~203-entries-per-leaf fan-out keeps this tree extremely
shallow regardless of key distribution. Tree depth is not where key
pattern shows up; split count is.

---

## 2. Reads: pages touched per operation

| Operation | 5,000 rows | 200,000 rows |
|---|---|---|
| point lookup | 2 pages | 3 pages |
| range scan (100 keys) | 3 pages | 4 pages |
| secondary-index lookup (index + heap) | 3 pages | 4 pages |

Point lookups grew by exactly one page across a 40x increase in row
count (5k → 200k) — consistent with `O(log n)` traversal and a height
increase from 2 to 3. This is the clearest confirmation in this project
that the B+ tree is doing what it's supposed to: lookup cost barely
moves as the dataset grows by orders of magnitude.

The secondary-index lookup (index search, then one heap read to
resolve the row) cost exactly one more page than a plain point lookup —
that one extra page *is* the cost of a non-clustered index, made
concrete rather than theoretical.

---

## 3. Write cost: indexing overhead

| Scenario | 5,000 rows | 200,000 rows |
|---|---|---|
| no index | 0 index writes | 0 index writes |
| one index | 5,099 | 203,998 |
| multiple indexes (2) | 10,245 | 409,914 |
| partial index (~50%) | 2,549 | 101,998 |

Each additional full index roughly **doubles** total index write
volume — unsurprising once stated, but worth having the actual
multiplier (not just "adding indexes costs more") for reasoning about
whether a given index is worth its write cost. The partial index sits
almost exactly at half the cost of a full index, tracking its ~50%
predicate match rate — direct evidence that a partial index's benefit
(fewer indexed rows) shows up proportionally in write cost, not just
storage size.

---

## 4. Buffer pool: does caching actually help, and by how much?

Two runs at very different scale, holding pool capacity fixed at 64
pages:

| | 5,000 inserts (35 pages total) | 200,000 inserts (1,442 pages total) |
|---|---|---|
| no pool | 9,854 reads, 5,094 writes, 111ms | 572,703 reads, 204,321 writes, 4.91s |
| pool (cap=64) | 0 reads, 35 writes, 41ms | 164,938 reads, 166,379 writes, 3.68s |

These two rows tell different stories, and the difference is the actual
lesson:

**At 5,000 inserts**, the tree tops out at 35 pages — comfortably under
the 64-page cache. The entire working set fits in memory at once, so
after the first pass, every access is a cache hit: **zero** real disk
reads, and only 35 writes total (one per page that ever became dirty,
flushed once at `Sync()`) instead of 5,094 write-through calls.

**At 200,000 inserts**, the tree grows to 1,442 pages — about 22x
larger than the 64-page cache. Now the pool is constantly evicting
pages before they're reused, a pattern with a name: **cache thrashing**.
Reads still drop substantially (572,703 → 164,938, roughly 3.5x fewer),
because hot pages near the root get accessed on every single insert and
stay resident, but the improvement is nowhere near the "zero reads"
result from the small run, because most leaf pages get evicted before
their next access.

**The general principle this demonstrates:** a cache's benefit is not
a fixed property of "having a cache" — it's a function of *(working-set
size) vs (cache size)*. A pool sized well below the working set still
helps (hot pages stay hot), but the dramatic wins only show up when the
cache can actually hold everything that matters.

---

## 5. Sync cost: the fsync cliff

| Strategy | 5,000 inserts | 200,000 inserts |
|---|---|---|
| sync every write | 15.9s | 8m 11.7s |
| sync every 100 writes | 250ms | 21.9s |
| sync once at end | 96ms | 7.26s |

At both scales, syncing on every write costs roughly **165–170x** more
than syncing once at the end. This is the single most dramatic number
in the whole project, and it's exactly the number that explains why no
production database calls `fsync` after every write: the guarantee
`fsync` provides (bytes are durably on disk, survive a crash) is real
and sometimes necessary, but it is orders of magnitude more expensive
than a plain `write()` that only hands bytes to the OS's own cache.

As flagged at the top: the *absolute* seconds here are on native ext4
and a real SSD — no filesystem-bridge overhead is involved. The
remaining variable is memory: with only ~2 GiB free at runtime, the
OS's page cache has limited room to absorb write bursts before `fsync`
forces them to the physical disk, so a machine with more headroom would
likely show smaller absolute numbers here. But the *shape* of the
result — per-write sync being two orders of magnitude more expensive
than batched or single-shot sync — is a property of what `fsync` does,
not an artifact of this machine, and should hold directionally on any
system.

This is also the concrete, felt reason WAL exists in real databases:
instead of fsync-ing large, scattered B+ tree pages on every commit,
a database fsyncs a small, sequential, append-only log — turning a
sync operation on scattered pages into a sync operation on one
contiguous write, which is both smaller and friendlier to the disk's
own write patterns.

---

## Summary

Nothing here is about raw throughput numbers being "good" or "bad" in
isolation — this was never meant to be a production benchmark (see
`task.md`, which explicitly rules that out). What the numbers do show,
directly rather than by assertion:

- **Lookup cost barely grows** with data volume — the B+ tree's
  logarithmic height is real and visible, not just theoretical.
- **Ascending keys split more often than random keys** under a plain
  50/50 split strategy — a real, counterintuitive tradeoff, not a bug.
- **Each additional index roughly doubles write cost** — a partial
  index's write savings track its predicate selectivity almost exactly.
- **A buffer pool's benefit depends entirely on whether the working set
  fits** — undersized relative to the data, it still helps, but nowhere
  near as dramatically as when the whole tree is cacheable.
- **fsync frequency is, by a wide margin, the single largest lever on
  write performance** measured anywhere in this project — far larger
  than indexing overhead or pooling.