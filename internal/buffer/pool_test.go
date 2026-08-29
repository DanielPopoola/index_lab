package buffer

import (
	"path/filepath"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

func newTestPool(t *testing.T, capacity int) (*Pool, *storage.PageManager, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pm, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}

	return NewPool(pm, capacity), pm, dbPath
}

func writeFreshPage(t *testing.T, pm *storage.PageManager) *page.Page {
	t.Helper()
	p := pm.AllocatePage()
	if err := pm.WritePage(p); err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}
	return p
}

func TestGetPageMissThenHit(t *testing.T) {
	pool, pm, _ := newTestPool(t, 4)
	p := writeFreshPage(t, pm)

	// First GetPage: not yet cached, must read through to pm.
	if _, err := pool.GetPage(p.ID); err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if pool.CacheMisses() != 1 {
		t.Errorf("CacheMisses = %d, want 1", pool.CacheMisses())
	}
	if pool.CacheHits() != 0 {
		t.Errorf("CacheHits = %d, want 0", pool.CacheHits())
	}

	// Second GetPage for the same page: should be served from cache.
	if _, err := pool.GetPage(p.ID); err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if pool.CacheMisses() != 1 {
		t.Errorf("CacheMisses after repeat access = %d, want still 1", pool.CacheMisses())
	}
	if pool.CacheHits() != 1 {
		t.Errorf("CacheHits = %d, want 1", pool.CacheHits())
	}
}

func TestEvictionPicksLeastRecentlyUsed(t *testing.T) {
	pool, pm, _ := newTestPool(t, 2)

	pA := writeFreshPage(t, pm)
	pB := writeFreshPage(t, pm)
	pC := writeFreshPage(t, pm)

	// Cache A, then B. Cache is now full: [B, A] (B most recent).
	if _, err := pool.GetPage(pA.ID); err != nil {
		t.Fatalf("GetPage(A) failed: %v", err)
	}
	if _, err := pool.GetPage(pB.ID); err != nil {
		t.Fatalf("GetPage(B) failed: %v", err)
	}

	// Touch A again, making B the least-recently-used: [A, B].
	if _, err := pool.GetPage(pA.ID); err != nil {
		t.Fatalf("GetPage(A) second access failed: %v", err)
	}

	// Bring in C. Cache is full, so this must evict B, not A.
	if _, err := pool.GetPage(pC.ID); err != nil {
		t.Fatalf("GetPage(C) failed: %v", err)
	}
	if pool.Evictions() != 1 {
		t.Fatalf("Evictions = %d, want 1", pool.Evictions())
	}

	if _, ok := pool.items[pB.ID]; ok {
		t.Errorf("expected B to be evicted, but it is still cached")
	}
	if _, ok := pool.items[pA.ID]; !ok {
		t.Errorf("expected A to still be cached (it was the most recently used)")
	}
	if _, ok := pool.items[pC.ID]; !ok {
		t.Errorf("expected C to be cached (just inserted)")
	}
}

func TestDirtyPageIsFlushedBeforeEviction(t *testing.T) {
	pool, pm, dbPath := newTestPool(t, 1)

	p := writeFreshPage(t, pm)
	loaded, err := pool.GetPage(p.ID)
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}

	entryBytes := []byte("0123456789012345") // 16-byte dummy entry
	loaded.InsertEntry(0, entryBytes)
	pool.MarkDirty(p.ID)

	// Force an eviction by requesting a second, different page while
	// capacity is 1.
	other := writeFreshPage(t, pm)
	if _, err := pool.GetPage(other.ID); err != nil {
		t.Fatalf("GetPage(other) failed: %v", err)
	}

	if pool.Evictions() != 1 {
		t.Fatalf("Evictions = %d, want 1", pool.Evictions())
	}
	if pool.DirtyFlushes() != 1 {
		t.Fatalf("DirtyFlushes = %d, want 1 (dirty page must be flushed on eviction)", pool.DirtyFlushes())
	}

	// Verify the mutation actually reached disk: open a second,
	// independent PageManager on the same file and read it back.
	verifyPM, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open (verify) failed: %v", err)
	}
	defer verifyPM.Close()

	onDisk, err := verifyPM.ReadPage(p.ID)
	if err != nil {
		t.Fatalf("ReadPage (verify) failed: %v", err)
	}
	if onDisk.NumEntries() != 1 {
		t.Errorf("on-disk page NumEntries = %d, want 1 (dirty write was lost)", onDisk.NumEntries())
	}
}

func TestCleanPageEvictedWithoutWrite(t *testing.T) {
	pool, pm, _ := newTestPool(t, 1)

	p := writeFreshPage(t, pm)
	if _, err := pool.GetPage(p.ID); err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	// Not mutated, not marked dirty.

	other := writeFreshPage(t, pm)
	if _, err := pool.GetPage(other.ID); err != nil {
		t.Fatalf("GetPage(other) failed: %v", err)
	}

	if pool.Evictions() != 1 {
		t.Fatalf("Evictions = %d, want 1", pool.Evictions())
	}
	if pool.DirtyFlushes() != 0 {
		t.Errorf("DirtyFlushes = %d, want 0 (clean page should not be written on eviction)", pool.DirtyFlushes())
	}
}

func TestCloseFlushesDirtyPages(t *testing.T) {
	pool, pm, dbPath := newTestPool(t, 4)

	p := writeFreshPage(t, pm)
	loaded, err := pool.GetPage(p.ID)
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}

	entryBytes := []byte("0123456789012345")
	loaded.InsertEntry(0, entryBytes)
	pool.MarkDirty(p.ID)

	if err := pool.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	verifyPM, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open (verify) failed: %v", err)
	}
	defer verifyPM.Close()

	onDisk, err := verifyPM.ReadPage(p.ID)
	if err != nil {
		t.Fatalf("ReadPage (verify) failed: %v", err)
	}
	if onDisk.NumEntries() != 1 {
		t.Errorf("on-disk page NumEntries = %d, want 1 (Close did not flush dirty page)", onDisk.NumEntries())
	}
}

func TestPutCachesPageNeverReadFromDisk(t *testing.T) {
	pool, pm, dbPath := newTestPool(t, 4)

	fresh := pm.AllocatePage()
	entryBytes := []byte("0123456789012345")
	fresh.InsertEntry(0, entryBytes)

	if err := pool.Put(fresh.ID, fresh); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Reading it back through the pool should return the same data
	// without touching disk (it's a cache hit on the entry Put created).
	got, err := pool.GetPage(fresh.ID)
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if got.NumEntries() != 1 {
		t.Fatalf("GetPage after Put: NumEntries = %d, want 1", got.NumEntries())
	}
	if pool.CacheMisses() != 0 {
		t.Errorf("CacheMisses = %d, want 0 (Put should have made this a hit)", pool.CacheMisses())
	}

	// Now flush it out and confirm it actually reaches disk — this is
	// the exact bug scenario: a page that MarkDirty alone would have
	// silently ignored, since it was never cached via GetPage.
	if err := pool.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	verifyPM, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open (verify) failed: %v", err)
	}
	defer verifyPM.Close()

	onDisk, err := verifyPM.ReadPage(fresh.ID)
	if err != nil {
		t.Fatalf("ReadPage (verify) failed: %v", err)
	}
	if onDisk.NumEntries() != 1 {
		t.Errorf("on-disk page NumEntries = %d, want 1 (page allocated-but-never-read was lost)", onDisk.NumEntries())
	}
}
