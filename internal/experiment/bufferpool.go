// Package experiment contains benchmark workloads for comparing cache and sync behavior.
package experiment

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/DanielPopoola/index_lab/internal/btree"
)

// BufferPoolResult reports the measured I/O and timing for one buffer-pool scenario.
type BufferPoolResult struct {
	Label       string // "no pool" or "pool (capacity=N)"
	Pattern     KeyPattern
	InsertCount int
	PageReads   uint64 // actual disk reads (0 for a write-only workload with no pool eviction pressure)
	PageWrites  uint64 // actual disk writes (pooled runs defer these; expect fewer than "no pool")
	Duration    time.Duration
}

// RunBufferPoolComparison benchmarks the same insert workload with and without a pool.
func RunBufferPoolComparison(dir string, pattern KeyPattern, n int, poolCapacity int) ([]BufferPoolResult, error) {
	keys := GenerateKeys(pattern, n)

	noPoolResult, err := runBufferPoolScenario(dir, "no pool", pattern, keys, nil)
	if err != nil {
		return nil, fmt.Errorf("no-pool scenario: %w", err)
	}

	pooledResult, err := runBufferPoolScenario(dir, fmt.Sprintf("pool (capacity=%d)", poolCapacity), pattern, keys, []btree.Option{btree.WithBufferPool(poolCapacity)})
	if err != nil {
		return nil, fmt.Errorf("pooled scenario: %w", err)
	}

	return []BufferPoolResult{noPoolResult, pooledResult}, nil
}

// runBufferPoolScenario measures one buffer-pool scenario for a given workload.
func runBufferPoolScenario(dir, label string, pattern KeyPattern, keys []int64, opts []btree.Option) (BufferPoolResult, error) {
	dbPath := filepath.Join(dir, fmt.Sprintf("bufferpool-%s-%s.db", pattern, sanitizeLabel(label)))

	tree, err := btree.Open(dbPath, opts...)
	if err != nil {
		return BufferPoolResult{}, err
	}
	defer tree.Close()

	start := time.Now()
	for _, key := range keys {
		if err := tree.Insert(key, key); err != nil {
			return BufferPoolResult{}, fmt.Errorf("inserting key %d: %w", key, err)
		}
	}
	// Sync (rather than just Close) so every deferred write from the
	// pool is flushed and counted before we read stats — otherwise a
	// pooled run could under-report PageWrites simply because pages
	// were still sitting dirty in the cache when we measured.
	if err := tree.Sync(); err != nil {
		return BufferPoolResult{}, err
	}
	duration := time.Since(start)

	stats, err := tree.Stats()
	if err != nil {
		return BufferPoolResult{}, err
	}

	return BufferPoolResult{
		Label:       label,
		Pattern:     pattern,
		InsertCount: len(keys),
		PageReads:   stats.PageReads,
		PageWrites:  stats.PageWrites,
		Duration:    duration,
	}, nil
}

// sanitizeLabel makes a label safe to use in a filename.
func sanitizeLabel(label string) string {
	out := make([]rune, 0, len(label))
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
