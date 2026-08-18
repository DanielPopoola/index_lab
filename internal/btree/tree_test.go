package btree

import (
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
