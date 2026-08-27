package btree

import (
	"bytes"
	"errors"
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

func TestDelete(t *testing.T) {
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

	if err := Delete(p, 20); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok := Search(p, 20)
	if ok {
		t.Fatalf("Deleted key still exists")
	}

	if p.NumEntries() != 2 {
		t.Fatalf("Expected %d entries but got: 2", p.NumEntries())
	}

	for _, e := range entries[:2] {
		gotRecordID, found := Search(p, e.key)
		if !found {
			t.Errorf("Search(p, %d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(p, %d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}

	err := Delete(p, 999)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Delete(999): err = %v, want ErrKeyNotFound", err)
	}

	if p.NumEntries() != 2 {
		t.Fatalf("Expected %d entries but got: 2", p.NumEntries())
	}
}

func TestRedistributeFromLeft(t *testing.T) {
	// Set up two adjacent leaves: leftSibling holds smaller keys,
	// underflowing holds larger keys. leftSibling has entries to
	// spare; underflowing does not (that's what makes it
	// "underflowing" in this scenario).
	leftSibling := page.NewLeafPage(0)
	underflowing := page.NewLeafPage(1)

	leftEntries := []struct{ key, recordID int64 }{
		{10, 100},
		{20, 200},
		{30, 300},
	}

	rightEntries := []struct{ key, recordID int64 }{
		{50, 500},
		{60, 600},
	}

	for _, e := range leftEntries {
		if err := Insert(leftSibling, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	for _, e := range rightEntries {
		if err := Insert(underflowing, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	newSeparator := redistributeFromLeft(underflowing, leftSibling)

	if leftSibling.NumEntries() != 2 {
		t.Fatalf("leftSibling.NumEntries() = %d, want 2", leftSibling.NumEntries())
	}
	if _, ok := Search(leftSibling, 30); ok {
		t.Errorf("Search(leftSibling, 30): expected found=false, got true")
	}

	if underflowing.NumEntries() != 3 {
		t.Fatalf("underflowing.NumEntries() = %d, want 3", underflowing.NumEntries())
	}
	gotRecordID, found := Search(underflowing, 30)
	if !found {
		t.Errorf("Search(underflowing, 30): expected found=true, got false")
	}
	if gotRecordID != 300 {
		t.Errorf("Search(underflowing, 30): recordID = %d, want 300", gotRecordID)
	}

	if !bytes.Equal(newSeparator, EncodeInt64(30)) {
		t.Errorf("newSeparator = %x, want %x", newSeparator, EncodeInt64(30))
	}

	// Sanity check: the move didn't disturb entries already there.
	for _, e := range rightEntries {
		gotRecordID, found := Search(underflowing, e.key)
		if !found {
			t.Errorf("Search(underflowing, %d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(underflowing, %d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}
}

func TestRedistributeFromRight(t *testing.T) {
	underflowing := page.NewLeafPage(0)
	rightSibling := page.NewLeafPage(1)

	leftEntries := []struct{ key, recordID int64 }{
		{10, 100},
		{20, 200},
	}

	rightEntries := []struct{ key, recordID int64 }{
		{30, 300},
		{40, 400},
		{50, 500},
	}

	for _, e := range leftEntries {
		if err := Insert(underflowing, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	for _, e := range rightEntries {
		if err := Insert(rightSibling, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	newSeparator := redistributeFromRight(underflowing, rightSibling)

	if rightSibling.NumEntries() != 2 {
		t.Fatalf("rightSibling.NumEntries() = %d, want 2", rightSibling.NumEntries())
	}
	if _, ok := Search(rightSibling, 30); ok {
		t.Errorf("Search(rightSibling, 30): expected found=false, got true")
	}

	if underflowing.NumEntries() != 3 {
		t.Fatalf("underflowing.NumEntries() = %d, want 3", underflowing.NumEntries())
	}
	gotRecordID, found := Search(underflowing, 30)
	if !found {
		t.Errorf("Search(underflowing, 30): expected found=true, got false")
	}
	if gotRecordID != 300 {
		t.Errorf("Search(underflowing, 30): recordID = %d, want 300", gotRecordID)
	}

	// Asymmetric case: the separator is rightSibling's NEW smallest
	// key (40), not the key that moved (30).
	if !bytes.Equal(newSeparator, EncodeInt64(40)) {
		t.Errorf("newSeparator = %x, want %x", newSeparator, EncodeInt64(40))
	}

	// Sanity check: the move didn't disturb entries already there.
	for _, e := range leftEntries {
		gotRecordID, found := Search(underflowing, e.key)
		if !found {
			t.Errorf("Search(underflowing, %d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(underflowing, %d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}
}

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
