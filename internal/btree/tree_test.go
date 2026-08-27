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

func TestMergeLeaf(t *testing.T) {
	// Three-leaf chain: left <-> right <-> rightsRight.
	// left and right are the two pages being merged; rightsRight
	// stands in for the "third page" whose PrevLeafPageID a real
	// caller would need to patch after this call (mergeLeaf itself
	// doesn't touch rightsRight — it only has left/right in hand).
	left := page.NewLeafPage(0)
	right := page.NewLeafPage(1)
	rightsRight := page.NewLeafPage(2)

	leftEntries := []struct{ key, recordID int64 }{
		{10, 100},
		{20, 200},
	}
	rightEntries := []struct{ key, recordID int64 }{
		{30, 300},
		{40, 400},
	}

	for _, e := range leftEntries {
		if err := Insert(left, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}
	for _, e := range rightEntries {
		if err := Insert(right, e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	// Wire up the chain as it would exist in a real tree before the merge.
	left.SetNextLeafPageID(right.ID)
	right.SetPrevLeafPageID(left.ID)
	right.SetNextLeafPageID(rightsRight.ID)
	rightsRight.SetPrevLeafPageID(right.ID)

	mergeLeaf(left, right)

	// left should now hold all four entries, in order.
	if left.NumEntries() != 4 {
		t.Fatalf("left.NumEntries() = %d, want 4", left.NumEntries())
	}
	allEntries := append(append([]struct{ key, recordID int64 }{}, leftEntries...), rightEntries...)
	for _, e := range allEntries {
		gotRecordID, found := Search(left, e.key)
		if !found {
			t.Errorf("Search(left, %d): expected found=true, got false", e.key)
			continue
		}
		if gotRecordID != e.recordID {
			t.Errorf("Search(left, %d): recordID = %d, want %d", e.key, gotRecordID, e.recordID)
		}
	}

	// right should now be empty.
	if right.NumEntries() != 0 {
		t.Fatalf("right.NumEntries() = %d, want 0 (right should be empty after merge)", right.NumEntries())
	}

	// left's chain pointer should now skip over right, pointing
	// straight at rightsRight. This is what a real caller reads to
	// know there's a third page needing its PrevLeafPageID patched.
	if left.NextLeafPageID() != rightsRight.ID {
		t.Errorf("left.NextLeafPageID() = %d, want %d (rightsRight)", left.NextLeafPageID(), rightsRight.ID)
	}
}

func TestFindChildIndex(t *testing.T) {
	// Internal page: leftmost=P1, entry0={key=30,child=P2}, entry1={key=60,child=P3}.
	leftmost := page.PageID(1)
	child2 := page.PageID(2)
	child3 := page.PageID(3)
	noSuchChild := page.PageID(99)

	p := page.NewInternalPage(0, leftmost)

	entry0 := append(EncodeInt64(30), encodeChildID(child2)...)
	p.InsertEntry(p.NumEntries(), entry0)

	entry1 := append(EncodeInt64(60), encodeChildID(child3)...)
	p.InsertEntry(p.NumEntries(), entry1)

	// Matches a real entry.
	idx, found := findChildIndex(p, child2)
	if !found {
		t.Errorf("findChildIndex(child2): expected found=true, got false")
	}
	if idx != 0 {
		t.Errorf("findChildIndex(child2): index = %d, want 0", idx)
	}

	idx, found = findChildIndex(p, child3)
	if !found {
		t.Errorf("findChildIndex(child3): expected found=true, got false")
	}
	if idx != 1 {
		t.Errorf("findChildIndex(child3): index = %d, want 1", idx)
	}

	// Matches the leftmost pointer, not a real entry — sentinel index
	// should be NumEntries(), not 0.
	idx, found = findChildIndex(p, leftmost)
	if !found {
		t.Errorf("findChildIndex(leftmost): expected found=true, got false")
	}
	if idx != p.NumEntries() {
		t.Errorf("findChildIndex(leftmost): index = %d, want %d (NumEntries sentinel)", idx, p.NumEntries())
	}

	// Matches nothing.
	_, found = findChildIndex(p, noSuchChild)
	if found {
		t.Errorf("findChildIndex(noSuchChild): expected found=false, got true")
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
