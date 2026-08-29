package btree

import (
	"path/filepath"
	"testing"
)

func TestStatsOnFreshTree(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	// Fresh tree: single empty leaf as root.
	stats, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Height != 1 {
		t.Fatalf("Height = %d, want 1", stats.Height)
	}
	if stats.TotalPages != 1 {
		t.Fatalf("TotalPages = %d, want 1", stats.TotalPages)
	}
	if stats.LeafPages != 1 {
		t.Fatalf("LeafPages = %d, want 1", stats.LeafPages)
	}
	if stats.InternalPages != 0 {
		t.Fatalf("InternalPages = %d, want 0", stats.InternalPages)
	}
	if stats.TotalEntries != 0 {
		t.Fatalf("TotalEntries = %d, want 0", stats.TotalEntries)
	}
	if stats.PageSplits != 0 {
		t.Fatalf("PageSplits = %d, want 0", stats.PageSplits)
	}
	if stats.PageMerges != 0 {
		t.Fatalf("PageMerges = %d, want 0", stats.PageMerges)
	}
}

func TestStatsCountsInsertsWithoutSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 5
	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	stats, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Height != 1 {
		t.Fatalf("Height = %d, want 1", stats.Height)
	}
	if stats.TotalEntries != numKeys {
		t.Fatalf("TotalEntries = %d, want %d", stats.TotalEntries, numKeys)
	}
	if stats.PageSplits != 0 {
		t.Fatalf("PageSplits = %d, want 0", stats.PageSplits)
	}
}

func TestStatsReflectsForcedSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 500
	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	stats, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Height < 2 {
		t.Fatalf("Height = %d, want >= 2 after %d inserts", stats.Height, numKeys)
	}
	if stats.LeafPages < 2 {
		t.Fatalf("LeafPages = %d, want >= 2 after forcing splits", stats.LeafPages)
	}
	if stats.InternalPages < 1 {
		t.Fatalf("InternalPages = %d, want >= 1", stats.InternalPages)
	}
	if stats.TotalPages != stats.LeafPages+stats.InternalPages {
		t.Fatalf("TotalPages = %d, want LeafPages(%d) + InternalPages(%d) = %d",
			stats.TotalPages, stats.LeafPages, stats.InternalPages, stats.LeafPages+stats.InternalPages)
	}
	if stats.TotalEntries != numKeys {
		t.Fatalf("TotalEntries = %d, want %d", stats.TotalEntries, numKeys)
	}
	if stats.PageSplits == 0 {
		t.Fatalf("PageSplits = 0, want > 0 after %d inserts", numKeys)
	}

	if stats.TotalPages <= 1 {
		t.Fatalf("TotalPages = %d, want > 1 after %d inserts", stats.TotalPages, numKeys)
	}
}

func TestStatsCountsMerges(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 500
	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	// Delete most keys to trigger merges.
	for i := int64(0); i < numKeys-10; i++ {
		if err := tree.Delete(i); err != nil {
			t.Fatalf("Delete(%d) failed: %v", i, err)
		}
	}

	stats, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.PageMerges == 0 {
		t.Fatalf("PageMerges = 0, want > 0 after deleting down to near-empty")
	}
	if stats.TotalEntries != 10 {
		t.Fatalf("TotalEntries = %d, want 10 remaining", stats.TotalEntries)
	}
}

func TestReadWriteCountersIncrement(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	statsBefore, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if err := tree.Insert(1, 100); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if _, found := tree.Search(1); !found {
		t.Fatalf("Search(1) not found")
	}

	statsAfter, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if statsAfter.PageReads <= statsBefore.PageReads {
		t.Fatalf("PageReads did not increase: before=%d, after=%d", statsBefore.PageReads, statsAfter.PageReads)
	}
	if statsAfter.PageWrites <= statsBefore.PageWrites {
		t.Fatalf("PageWrites did not increase: before=%d, after=%d", statsBefore.PageWrites, statsAfter.PageWrites)
	}
}

func TestResetStatsZeroesCountersNotStructure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 500
	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	beforeReset, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if beforeReset.PageSplits == 0 {
		t.Fatalf("setup problem: expected some splits before reset")
	}

	tree.ResetStats()

	afterReset, err := tree.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if afterReset.PageSplits != 0 {
		t.Fatalf("PageSplits after ResetStats = %d, want 0", afterReset.PageSplits)
	}
	if afterReset.PageMerges != 0 {
		t.Fatalf("PageMerges after ResetStats = %d, want 0", afterReset.PageMerges)
	}
	if afterReset.PageReads != 0 && afterReset.PageReads > 10 {
		// A couple of reads are expected from Stats() walking the tree
		// itself just now — but it must not carry forward the hundreds
		// of reads from the original 500 inserts.
		t.Fatalf("PageReads after ResetStats = %d, want close to 0 (only from Stats' own walk)", afterReset.PageReads)
	}

	// Structural facts must be untouched by ResetStats — the tree
	// itself didn't change shape, only the event counters reset.
	if afterReset.Height != beforeReset.Height {
		t.Fatalf("Height changed after ResetStats: before=%d, after=%d", beforeReset.Height, afterReset.Height)
	}
	if afterReset.TotalEntries != beforeReset.TotalEntries {
		t.Fatalf("TotalEntries changed after ResetStats: before=%d, after=%d", beforeReset.TotalEntries, afterReset.TotalEntries)
	}
}
