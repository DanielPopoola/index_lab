package btree

import (
	"path/filepath"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// TestBTreeSurvivesReopen proves the full stack works end to end, through
// the public BTree API (not reaching into page/storage directly): insert
// keys, close the tree, reopen the SAME file, and confirm every key is
// still searchable with the correct recordID. This is the same shape as
// storage's TestPageSurvivesReopen, one layer up.
func TestBTreeSurvivesReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	entries := []struct{ key, recordID int64 }{
		{30, 300},
		{10, 100},
		{20, 200},
	}

	tree1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for _, e := range entries {
		if err := tree1.Insert(e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	if err := tree1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tree2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree2.Close()

	for _, e := range entries {
		expectedRecordID, found := tree2.Search(e.key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", e.key)
			continue
		}
		if expectedRecordID != e.recordID {
			t.Errorf("Search(%d): recordID = %d, want %d", e.key, expectedRecordID, e.recordID)
		}
	}
}

// TestInsertTriggersSplit inserts enough keys to force exactly ONE leaf
// split (root leaf -> root internal page + 2 leaves), then confirms every
// key is still correctly searchable through the resulting structure.
//
// SCOPE NOTE: this deliberately stays within a single split. A SECOND
// split (once the root is already an internal page) isn't handled yet —
// insertWithSplit only knows how to promote a brand new root from a
// leaf-was-root state. Inserting enough keys to force multiple splits
// will currently break; that's the next thing to build, not tonight.
func TestInsertTriggersSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 220

	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("expected root to be InternalPage after %d inserts (a split should have happened), got PageType=%v", numKeys, rootPage.PageType())
	}

	for i := int64(0); i < numKeys; i++ {
		gotRecordID, found := tree.Search(i)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", i)
			continue
		}
		if gotRecordID != i*10 {
			t.Errorf("Search(%d): recordID = %d, want %d", i, gotRecordID, i*10)
		}
	}

	if _, found := tree.Search(99999); found {
		t.Errorf("Search(99999): expected found=false, got true")
	}
}

// TestInsertTriggersMultiLevelSplit forces a SECOND level of splitting:
// enough leaf splits happen that the root (now an internal page) itself
// overflows and has to split too, requiring propagateSplit to recurse
// past a single level (leaf -> parent) and instead build a brand new
// root above two internal pages.
//
// A page holds ~203 entries (4096 - 21 header) / (16 entry + 4 slot).
// That's true for both leaf AND internal pages, since internal entries
// are also 16 bytes (8 key + 8 childID). So forcing the ROOT itself to
// split needs roughly 203 leaf splits' worth of keys, i.e. ~203 * 203.
// Rounding up generously to be safely past that threshold.
func TestInsertTriggersMultiLevelSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 45000

	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("expected root to be InternalPage after %d inserts, got PageType=%v", numKeys, rootPage.PageType())
	}

	// The real signal that a SECOND-level split happened: the root's
	// leftmost child should itself be an internal page, not a leaf.
	// If only one split ever happened, root -> leaf directly.
	leftmostChild, err := tree.pm.ReadPage(rootPage.LeftmostChildPageID())
	if err != nil {
		t.Fatalf("ReadPage(root's leftmost child) failed: %v", err)
	}
	if leftmostChild.PageType() != page.InternalPage {
		t.Fatalf("expected root's leftmost child to be InternalPage (proof of a second split level), got PageType=%v", leftmostChild.PageType())
	}

	// Spot-check correctness rather than searching all 45000 keys, to
	// keep the test fast: first, last, middle, and a few scattered.
	spotChecks := []int64{0, 1, numKeys / 4, numKeys / 2, (3 * numKeys) / 4, numKeys - 1}
	for _, key := range spotChecks {
		gotRecordID, found := tree.Search(key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", key)
			continue
		}
		if gotRecordID != key*10 {
			t.Errorf("Search(%d): recordID = %d, want %d", key, gotRecordID, key*10)
		}
	}

	if _, found := tree.Search(numKeys + 999); found {
		t.Errorf("Search(%d): expected found=false, got true", numKeys+999)
	}
}
