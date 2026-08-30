// Package experiment contains benchmark workloads for comparing cache and sync behavior.
package experiment

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/DanielPopoola/index_lab/internal/btree"
)

// SyncCostResult reports the timing and Sync-call count for one fsync strategy.
type SyncCostResult struct {
	Strategy    string // e.g. "sync every write", "sync every 100 writes", "sync once at end"
	InsertCount int
	SyncCalls   uint64
	Duration    time.Duration
}

// RunSyncCostExperiments compares Sync frequency against the same insert workload.
func RunSyncCostExperiments(dir string, n int, syncInterval int) ([]SyncCostResult, error) {
	keys := GenerateKeys(Sequential, n)

	everyWrite, err := runSyncScenario(dir, "sync every write", keys, 1)
	if err != nil {
		return nil, fmt.Errorf("sync-every-write scenario: %w", err)
	}

	everyInterval, err := runSyncScenario(dir, fmt.Sprintf("sync every %d writes", syncInterval), keys, syncInterval)
	if err != nil {
		return nil, fmt.Errorf("sync-every-interval scenario: %w", err)
	}

	// interval > len(keys) guarantees no in-loop sync fires, so only
	// the final Sync call after the loop counts.
	onceAtEnd, err := runSyncScenario(dir, "sync once at end", keys, len(keys)+1)
	if err != nil {
		return nil, fmt.Errorf("sync-once-at-end scenario: %w", err)
	}

	return []SyncCostResult{everyWrite, everyInterval, onceAtEnd}, nil
}

// runSyncScenario measures one Sync strategy for a given workload.
func runSyncScenario(dir, label string, keys []int64, syncInterval int) (SyncCostResult, error) {
	dbPath := filepath.Join(dir, fmt.Sprintf("synccost-%s.db", sanitizeLabel(label)))

	tree, err := btree.Open(dbPath)
	if err != nil {
		return SyncCostResult{}, err
	}
	defer tree.Close()

	var syncCalls uint64
	start := time.Now()
	for i, key := range keys {
		if err := tree.Insert(key, key); err != nil {
			return SyncCostResult{}, fmt.Errorf("inserting key %d: %w", key, err)
		}
		if (i+1)%syncInterval == 0 {
			if err := tree.Sync(); err != nil {
				return SyncCostResult{}, err
			}
			syncCalls++
		}
	}
	// Final sync to guarantee durability of whatever tail didn't land
	// on a syncInterval boundary — without this, "sync once at end"
	// would otherwise never sync at all when syncInterval > len(keys).
	if err := tree.Sync(); err != nil {
		return SyncCostResult{}, err
	}
	syncCalls++
	duration := time.Since(start)

	return SyncCostResult{
		Strategy:    label,
		InsertCount: len(keys),
		SyncCalls:   syncCalls,
		Duration:    duration,
	}, nil
}
