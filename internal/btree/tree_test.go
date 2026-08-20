package btree

import (
	"bytes"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
)

func TestInsertAndSearch(t *testing.T) {
	p := page.NewLeafPage(0)

	entries := []struct{ key, recordID int64 }{
		{30, 300},
		{10, 100},
		{20, 200},
	}

	for _, e := range entries {
		if err := Insert(p, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	for _, e := range entries {
		gotRecordID, found := Search(p, e.key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(%d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}

	_, found := Search(p, 999)
	if found {
		t.Errorf("Search(999): expected found=false, got true")
	}

	// Sanity check: did all 3 inserts actually land on the page?
	if p.NumEntries() != 3 {
		t.Errorf("NumEntries() = %d, want 3", p.NumEntries())
	}

}

// TestSplitLeaf proves splitLeaf correctly divides a full leaf's entries,
// wires up sibling pointers, and returns the correct separator key —
// using a FAKE allocator, so this test needs no real file/PageManager.
func TestSplitLeaf(t *testing.T) {
	oldPage := page.NewLeafPage(0)

	entries := []struct{ key, recordID int64 }{
		{10, 100},
		{20, 200},
		{30, 300},
		{40, 400},
	}

	for _, e := range entries {
		if err := Insert(oldPage, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	nextID := page.PageID(1)
	fakeAllocate := func() *page.Page {
		p := page.NewLeafPage(nextID)
		nextID++
		return p
	}

	separatorKey, newPage, err := splitLeaf(oldPage, fakeAllocate)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if !bytes.Equal(separatorKey, EncodeInt64(30)) {
		t.Errorf("separatorKey = %v, want %v (encoded 30)", separatorKey, EncodeInt64(30))
	}

	if oldPage.NumEntries() != 2 {
		t.Errorf("NumEntries for oldPage: %d, want 2", oldPage.NumEntries())
	}

	if newPage.NumEntries() != 2 {
		t.Errorf("NumEntries for newPage: %d, want 2", newPage.NumEntries())
	}

	for _, e := range entries[:2] {
		_, found := Search(oldPage, e.key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", e.key)
			continue
		}
	}

	for _, e := range entries[2:] {
		gotRecordID, found := Search(newPage, e.key)
		if !found {
			t.Errorf("Search(newPage, %d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(newPage, %d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}

	if oldPage.NextLeafPageID() != newPage.ID {
		t.Errorf("oldPage.NextLeafPageID() = %d, want %d", oldPage.NextLeafPageID(), newPage.ID)
	}
	if oldPage.PrevLeafPageID() != 0 {
		t.Errorf("oldPage.PrevLeafPageID = %d, want %d", oldPage.PrevLeafPageID(), 0)
	}
	if newPage.PrevLeafPageID() != oldPage.ID {
		t.Errorf("newPage.PrevLeafPageID() = %d, want %d", newPage.PrevLeafPageID(), oldPage.ID)
	}
	if newPage.NextLeafPageID() != 0 {
		t.Errorf("newPage.NextLeafPageID() = %d, want %d", newPage.NextLeafPageID(), 0)
	}
}
