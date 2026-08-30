# B+ Tree Index Laboratory

## Project Goal

Implement a small, persistent **B+ tree index in Go** to understand how
database indexes work internally and how index structure affects read
and write behavior.

This project is an indexing laboratory, not a database implementation.
It will implement only the storage and indexing machinery required to
experiment with B+ tree behavior.

The implementation must use fixed-size pages and page IDs rather than
ordinary in-memory tree nodes containing direct pointers.

------------------------------------------------------------------------

# Scope

## 1. Page-Based Storage

Implement a simple page manager backed by a single database file.

### Requirements

-   Use a configurable fixed page size.
-   The default page size must be **4 KiB (4096 bytes)**.
-   Every page has a numeric `PageID`.
-   A page is located in the file using its `PageID` and the configured
    page size.
-   Pages must be serialized to and deserialized from raw bytes.
-   The page manager must support:
    -   creating/allocating a page;
    -   reading a page by `PageID`;
    -   writing a page by `PageID`;
    -   reusing an existing page after it has been loaded.
-   Do not implement a general-purpose filesystem or storage engine.

### Page abstraction

A page should contain only the metadata and bytes required by the B+
tree.

The implementation must not store Go pointers to other B+ tree nodes as
the persistent representation.

Instead, child references must be represented by `PageID`s.

------------------------------------------------------------------------

# 2. B+ Tree Structure

Implement a **B+ tree**, not a generic binary search tree and not a
traditional in-memory B-tree.

The tree must have:

-   one root page;
-   internal pages containing separator keys and child `PageID`s;
-   leaf pages containing index entries;
-   a linked list between leaf pages.

Conceptually:

``` text
                         Root
                    [30 | 60]
                   /    |    \
                  /     |     \
             Leaf 1   Leaf 2   Leaf 3
             [10,20]  [30,40]  [60,70]
                 ↔        ↔        ↔
```

All actual index entries must reside in leaf pages.

Internal pages are used only to direct searches.

------------------------------------------------------------------------

# 3. Index Entry Model

The initial index must be a **single-column integer index**.

Use:

``` text
Key: int64
Value: RecordID
```

`RecordID` should be represented as a stable reference to a row stored
outside the index. For this project, it may simply be a numeric
identifier.

Do not implement arbitrary SQL data types.

Do not implement strings, JSON, dates, or user-defined types in the
first version.

The initial B+ tree therefore supports:

``` text
int64 key → RecordID
```

------------------------------------------------------------------------

# 4. B+ Tree Operations

Implement the following operations:

``` text
Insert(key, recordID)
Search(key)
Delete(key)
```

## Insert

Insertion must:

1.  Traverse from the root to the appropriate leaf.
2.  Insert the key in sorted order.
3.  Split a leaf when it exceeds its capacity.
4.  Propagate the separator key into the parent.
5.  Split internal pages when necessary.
6.  Create a new root when the existing root splits.

## Search

Search must:

1.  Start at the root.
2.  Follow child `PageID`s through internal pages.
3.  Locate the appropriate leaf.
4.  Search the leaf for the requested key.
5.  Return the associated `RecordID` if the key exists.

## Delete

Implement deletion with:

-   removal from the leaf;
-   redistribution from a sibling when appropriate;
-   page merging when redistribution cannot maintain the required
    occupancy;
-   propagation of structural changes toward the root;
-   root reduction when the root no longer requires multiple levels.

Do not implement advanced deletion optimizations beyond what is required
to maintain a valid B+ tree.

------------------------------------------------------------------------

# 5. Leaf-Page Linking

Every leaf page must contain references to its neighboring leaf pages.

At minimum:

``` text
previousLeafPageID
nextLeafPageID
```

This allows a range scan to move sequentially between leaf pages.

The links must remain correct after:

-   insertion;
-   leaf splitting;
-   deletion;
-   leaf merging.

------------------------------------------------------------------------

# 6. Range Scans

Implement:

``` text
Scan(startKey, endKey)
```

The operation must:

1.  Find the leaf containing the beginning of the range.
2.  Scan entries within that leaf.
3.  Follow `nextLeafPageID`.
4.  Continue until the end of the requested key range.

The result must be returned in ascending key order.

This operation exists specifically to demonstrate why B+ trees are
useful for ordered and range queries.

------------------------------------------------------------------------

# 7. Secondary Index Model

The B+ tree implemented in this project is a **secondary/non-clustered
index model**.

The leaf contains:

``` text
key → RecordID
```

It must **not** contain the complete database row.

Create a minimal separate heap representation only to demonstrate the
extra lookup:

``` text
B+ Tree
   │
   │ key → RecordID
   ↓
Heap
   │
   ↓
Row
```

The heap does not need to support general SQL operations.

It only needs to allow a `RecordID` returned by the index to resolve to
a stored row.

The purpose is to measure and understand the additional lookup performed
by a secondary index.

------------------------------------------------------------------------

# 8. Composite Index

Extend the same B+ tree implementation to support a two-column composite
key.

The key must be:

``` text
(columnA, columnB)
```

Both columns should initially use `int64`.

Keys must be ordered lexicographically:

``` text
(1, 10)
(1, 20)
(1, 30)
(2, 10)
(2, 20)
(3, 10)
```

Implement:

``` text
Search(columnA, columnB)
Scan(columnA, startColumnB, endColumnB)
```

The implementation must demonstrate that:

``` text
(columnA, columnB)
```

can efficiently locate ranges constrained by `columnA`, while a search
constrained only by `columnB` cannot directly locate one contiguous
region of the tree.

Do not implement a general query optimizer.

------------------------------------------------------------------------

# 9. Unique Index

Add an optional uniqueness constraint to the single-column integer
index.

For a unique index:

``` text
Insert(key, recordID)
```

must fail if the key already exists.

The uniqueness check must be performed as part of the B+ tree insertion
operation.

Do not implement SQL constraint syntax.

------------------------------------------------------------------------

# 10. Partial Index

Add support for a partial index using one fixed predicate:

``` text
status == ACTIVE
```

Use a minimal row model containing:

``` text
RecordID
Key
Status
```

The B+ tree must contain an entry only when:

``` text
Status == ACTIVE
```

For example:

``` text
Record 1: key=10, status=ACTIVE   → indexed
Record 2: key=20, status=DELETED  → not indexed
Record 3: key=30, status=ACTIVE   → indexed
```

When a record changes from `DELETED` to `ACTIVE`, the index must add the
record.

When a record changes from `ACTIVE` to `DELETED`, the index must remove
the record.

Do not implement arbitrary user-defined predicates.

------------------------------------------------------------------------

# 11. Index Statistics

The implementation must expose basic statistics for experiments.

Track at least:

``` text
tree height
number of pages
number of leaf pages
number of internal pages
number of entries
page splits
page merges
page reads
page writes
```

These statistics should be resettable between experiments.

------------------------------------------------------------------------

# 12. Workload Experiments

Create a small benchmark/experiment program that compares the behavior
of the index under different key-generation patterns.

Test at least:

### Sequential integer keys

``` text
1
2
3
4
...
```

### Random integer keys

Generate uniformly distributed random `int64` keys.

### UUID4-like random keys

Use randomly generated 128-bit values as the key representation for a
separate experiment.

### UUID7-like ordered keys

Use time-ordered 128-bit values for a separate experiment.

The purpose is **not** to implement the UUID standards.

The experiment only needs representative random and time-ordered 128-bit
key distributions.

For each workload, record:

-   insertion count;
-   page splits;
-   page writes;
-   final tree height;
-   final page count.

Do not make claims about real database performance based solely on this
benchmark. The experiment is intended to demonstrate B+ tree locality
and page-splitting behavior in this implementation.

------------------------------------------------------------------------

# 13. Read Experiments

Measure the difference between:

### Point lookup

``` text
Search(key)
```

### Range lookup

``` text
Scan(startKey, endKey)
```

### Secondary-index lookup

``` text
index lookup
    ↓
RecordID
    ↓
heap lookup
```

Record:

-   number of index pages read;
-   number of heap pages read;
-   total pages touched.

The purpose is to develop intuition for how index structure affects page
access.

------------------------------------------------------------------------

# 14. Write Cost Experiments

Compare the write behavior of:

1.  no index;
2.  one B+ tree index;
3.  multiple B+ tree indexes;
4.  a partial index.

Measure:

-   number of index page writes;
-   page splits;
-   page merges;
-   total pages affected.

The goal is to demonstrate that indexes improve some reads while adding
maintenance work to writes.

------------------------------------------------------------------------

# 15. Buffer Pool

Implement a small buffer pool between the B+ tree and the page manager.

The buffer pool must:

-   cache pages by `PageID`;
-   track dirty pages;
-   return cached pages without rereading the file;
-   evict pages when the pool is full;
-   write dirty pages back before eviction.

Use a simple **LRU eviction policy**.

The buffer pool should expose:

``` text
cache hits
cache misses
evictions
dirty-page flushes
```

Do not implement advanced replacement algorithms.

------------------------------------------------------------------------

# 16. Durability Boundary

Add minimal write synchronization to demonstrate the relationship
between an index page and persistent storage.

The project must provide an explicit operation equivalent to:

``` text
write page
sync file
```

The implementation must distinguish between:

``` text
bytes written to the OS/file
```

and:

``` text
a request to synchronize the file's contents to durable storage
```

Do not implement a full WAL or crash-recovery system in this project.

WAL is explicitly out of scope.

------------------------------------------------------------------------

# 17. Concurrency

Concurrency is **out of scope for the implementation**.

Do not implement:

-   row locks;
-   page latches;
-   next-key locks;
-   gap locks;
-   MVCC;
-   transaction isolation;
-   phantom prevention.

These concepts may be studied separately after the index is complete,
but they are not part of this project.

------------------------------------------------------------------------

# 18. Explicitly Out of Scope

Do not implement any of the following:

-   SQL parser;
-   SQL query planner;
-   query optimizer;
-   SQL execution engine;
-   transactions;
-   MVCC;
-   WAL;
-   crash recovery;
-   replication;
-   sharding;
-   distributed consensus;
-   arbitrary data types;
-   arbitrary partial-index predicates;
-   full clustered-table storage;
-   database networking;
-   authentication;
-   user management;
-   production-grade durability guarantees;
-   production-grade benchmarking.

The project is an **index and page-storage laboratory**, not a
general-purpose database.

------------------------------------------------------------------------

# 19. Suggested Package Structure

Use a small, feature-oriented Go project.

A reasonable starting structure is:

``` text
index-lab/
├── cmd/
│   └── indexlab/
│       └── main.go
│
├── internal/
│   ├── page/
│   │   └── page.go
│   ├── storage/
│   │   └── file.go
│   ├── buffer/
│   │   └── pool.go
│   ├── btree/
│   │   ├── tree.go
│   │   ├── node.go
│   │   └── key.go
│   ├── heap/
│   │   └── heap.go
│   └── experiment/
│       └── experiment.go
│
├── tests/
│   └── ...
│
├── go.mod
└── task.md
```

The exact package structure may change if a simpler design becomes
clearer, but the implementation should remain separated by
responsibility.

------------------------------------------------------------------------

# 20. Completion Criteria

The project is complete when all of the following are true:

-   [ ] B+ tree pages are fixed-size and persisted to a file.
-   [ ] Persistent child references use `PageID`s rather than Go
    pointers.
-   [ ] Search works after reopening the database file.
-   [ ] Insert works across multiple tree levels.
-   [ ] Leaf splits work.
-   [ ] Internal-page splits work.
-   [ ] Root splits work.
-   [ ] Delete maintains B+ tree invariants.
-   [ ] Leaf pages are linked correctly.
-   [ ] Range scans work in ascending order.
-   [ ] Secondary index lookups resolve through a `RecordID`.
-   [ ] Composite `(columnA, columnB)` keys work.
-   [ ] Unique indexes reject duplicate keys.
-   [ ] Partial indexes include only rows satisfying `status == ACTIVE`.
-   [ ] Index statistics are collected.
-   [ ] Buffer-pool caching works.
-   [ ] Dirty pages are flushed on eviction.
-   [ ] File synchronization can be explicitly requested.
-   [ ] Sequential, random, and time-ordered key experiments can be run.
-   [ ] The experiments report page-level statistics.

------------------------------------------------------------------------

# Final Learning Objective

At the end of the project, you should be able to look at a query
workload and reason about an index using the following chain:

``` text
Query pattern
     ↓
Key structure
     ↓
B+ tree ordering
     ↓
Contiguous key range?
     ↓
Pages touched
     ↓
Buffer-pool behavior
     ↓
Heap lookup required?
     ↓
Index maintenance on writes
     ↓
Is the index actually worthwhile?
```

The goal is not to reproduce PostgreSQL or MySQL.

The goal is to understand **why a database chooses a particular index
structure for a particular workload**.
