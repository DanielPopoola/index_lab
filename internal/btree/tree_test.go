package btree

import (
	"bytes"
	"encoding/binary"
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

func buildInternalPage(id, leftmost page.PageID, entries []struct {
	key   int64
	child page.PageID
}) *page.Page {
	p := page.NewInternalPage(id, leftmost)
	for _, e := range entries {
		entryBytes := append(EncodeInt64(e.key), encodeChildID(e.child)...)
		p.InsertEntry(p.NumEntries(), entryBytes)
	}
	return p
}

func TestFindSiblingPageIDs(t *testing.T) {
	// grandparent = [Leftmost=C0 | key=30,child=C1 | key=60,child=C2]
	c0, c1, c2 := page.PageID(10), page.PageID(11), page.PageID(12)
	grandparent := buildInternalPage(0, c0, []struct {
		key   int64
		child page.PageID
	}{
		{30, c1},
		{60, c2},
	})

	// node = C0 (leftmost): no left sibling, right sibling = C1.
	nodeC0 := &page.Page{ID: c0}
	leftID, rightID := findSiblingPageIDs(grandparent, nodeC0)
	if leftID != 0 {
		t.Errorf("C0: leftID = %d, want 0 (none)", leftID)
	}
	if rightID != c1 {
		t.Errorf("C0: rightID = %d, want %d (C1)", rightID, c1)
	}

	// node = C1 (middle): left sibling = C0, right sibling = C2.
	nodeC1 := &page.Page{ID: c1}
	leftID, rightID = findSiblingPageIDs(grandparent, nodeC1)
	if leftID != c0 {
		t.Errorf("C1: leftID = %d, want %d (C0)", leftID, c0)
	}
	if rightID != c2 {
		t.Errorf("C1: rightID = %d, want %d (C2)", rightID, c2)
	}

	// node = C2 (last): left sibling = C1, no right sibling.
	nodeC2 := &page.Page{ID: c2}
	leftID, rightID = findSiblingPageIDs(grandparent, nodeC2)
	if leftID != c1 {
		t.Errorf("C2: leftID = %d, want %d (C1)", leftID, c1)
	}
	if rightID != 0 {
		t.Errorf("C2: rightID = %d, want 0 (none)", rightID)
	}
}

func TestRedistributeInternalFromLeft(t *testing.T) {
	// leftSibling = [leftmost=X | key=20,child=Y]  (2 children: X, Y)
	// underflowing = [leftmost=Z | key=80,child=W]  (2 children: Z, W)
	// old parent separator between them: 50.
	x, y, z, w := page.PageID(1), page.PageID(2), page.PageID(3), page.PageID(4)

	leftSibling := buildInternalPage(0, x, []struct {
		key   int64
		child page.PageID
	}{{20, y}})

	underflowing := buildInternalPage(1, z, []struct {
		key   int64
		child page.PageID
	}{{80, w}})

	oldParentSeparator := EncodeInt64(50)

	newParentSeparator := redistributeInternalFromLeft(underflowing, leftSibling, oldParentSeparator)

	// leftSibling loses its last entry: back to just leftmost=X.
	if leftSibling.NumEntries() != 0 {
		t.Fatalf("leftSibling.NumEntries() = %d, want 0", leftSibling.NumEntries())
	}
	if leftSibling.LeftmostChildPageID() != x {
		t.Errorf("leftSibling.LeftmostChildPageID() = %d, want %d (X, unchanged)", leftSibling.LeftmostChildPageID(), x)
	}

	// underflowing: new leftmost is Y (the moved child); new first
	// entry pairs the OLD parent separator (50) with underflowing's
	// OLD leftmost (Z).
	if underflowing.NumEntries() != 2 {
		t.Fatalf("underflowing.NumEntries() = %d, want 2", underflowing.NumEntries())
	}
	if underflowing.LeftmostChildPageID() != y {
		t.Errorf("underflowing.LeftmostChildPageID() = %d, want %d (Y, the moved child)", underflowing.LeftmostChildPageID(), y)
	}
	firstEntry := underflowing.GetEntry(0)
	if !bytes.Equal(firstEntry[:8], EncodeInt64(50)) {
		t.Errorf("underflowing entry 0 key = %x, want %x (old parent separator)", firstEntry[:8], EncodeInt64(50))
	}
	firstEntryChild := page.PageID(binary.BigEndian.Uint64(firstEntry[8:]))
	if firstEntryChild != z {
		t.Errorf("underflowing entry 0 child = %d, want %d (Z, underflowing's old leftmost)", firstEntryChild, z)
	}

	// new parent separator = the moved entry's own key (20).
	if !bytes.Equal(newParentSeparator, EncodeInt64(20)) {
		t.Errorf("newParentSeparator = %x, want %x (20)", newParentSeparator, EncodeInt64(20))
	}
}

func TestRedistributeInternalFromRight(t *testing.T) {
	// underflowing = [leftmost=P | key=10,child=Q]  (2 children: P, Q)
	// rightSibling = [leftmost=R | key=80,child=S]  (2 children: R, S)
	// old parent separator between them: 50.
	p, q, r, s := page.PageID(1), page.PageID(2), page.PageID(3), page.PageID(4)

	underflowing := buildInternalPage(0, p, []struct {
		key   int64
		child page.PageID
	}{{10, q}})

	rightSibling := buildInternalPage(1, r, []struct {
		key   int64
		child page.PageID
	}{{80, s}})

	oldParentSeparator := EncodeInt64(50)

	newParentSeparator := redistributeInternalFromRight(underflowing, rightSibling, oldParentSeparator)

	// rightSibling: R moved out, new leftmost is S.
	if rightSibling.NumEntries() != 0 {
		t.Fatalf("rightSibling.NumEntries() = %d, want 0", rightSibling.NumEntries())
	}
	if rightSibling.LeftmostChildPageID() != s {
		t.Errorf("rightSibling.LeftmostChildPageID() = %d, want %d (S)", rightSibling.LeftmostChildPageID(), s)
	}

	// underflowing: gains a new LAST entry pairing the OLD parent
	// separator (50) with the moved child (R). Leftmost (P) unchanged.
	if underflowing.NumEntries() != 2 {
		t.Fatalf("underflowing.NumEntries() = %d, want 2", underflowing.NumEntries())
	}
	if underflowing.LeftmostChildPageID() != p {
		t.Errorf("underflowing.LeftmostChildPageID() = %d, want %d (P, unchanged)", underflowing.LeftmostChildPageID(), p)
	}
	lastEntry := underflowing.GetEntry(underflowing.NumEntries() - 1)
	if !bytes.Equal(lastEntry[:8], EncodeInt64(50)) {
		t.Errorf("underflowing last entry key = %x, want %x (old parent separator)", lastEntry[:8], EncodeInt64(50))
	}
	lastEntryChild := page.PageID(binary.BigEndian.Uint64(lastEntry[8:]))
	if lastEntryChild != r {
		t.Errorf("underflowing last entry child = %d, want %d (R, the moved child)", lastEntryChild, r)
	}

	// new parent separator = rightSibling's NEW first entry's key (80)
	// — NOT the moved entry. Same asymmetry as leaf redistribute-from-right.
	if !bytes.Equal(newParentSeparator, EncodeInt64(80)) {
		t.Errorf("newParentSeparator = %x, want %x (80)", newParentSeparator, EncodeInt64(80))
	}
}

func TestMergeInternal(t *testing.T) {
	// left = [leftmost=P1 | key=30,child=P2]
	// right = [leftmost=P3 | key=150,child=P4]
	// parent separator between them: 100.
	p1, p2, p3, p4 := page.PageID(1), page.PageID(2), page.PageID(3), page.PageID(4)

	left := buildInternalPage(0, p1, []struct {
		key   int64
		child page.PageID
	}{{30, p2}})

	right := buildInternalPage(1, p3, []struct {
		key   int64
		child page.PageID
	}{{150, p4}})

	oldParentSeparator := EncodeInt64(100)

	mergeInternal(left, right, oldParentSeparator)

	// Expected result: left = [leftmost=P1 | key=30,child=P2 | key=100,child=P3 | key=150,child=P4]
	if left.NumEntries() != 3 {
		t.Fatalf("left.NumEntries() = %d, want 3", left.NumEntries())
	}
	if left.LeftmostChildPageID() != p1 {
		t.Errorf("left.LeftmostChildPageID() = %d, want %d (P1, unchanged)", left.LeftmostChildPageID(), p1)
	}

	entry0 := left.GetEntry(0)
	if !bytes.Equal(entry0[:8], EncodeInt64(30)) {
		t.Errorf("entry 0 key = %x, want %x (30, left's original entry)", entry0[:8], EncodeInt64(30))
	}
	if child := page.PageID(binary.BigEndian.Uint64(entry0[8:])); child != p2 {
		t.Errorf("entry 0 child = %d, want %d (P2)", child, p2)
	}

	// The synthetic entry: old parent separator (100) paired with
	// right's old leftmost (P3).
	entry1 := left.GetEntry(1)
	if !bytes.Equal(entry1[:8], EncodeInt64(100)) {
		t.Errorf("entry 1 key = %x, want %x (100, pulled-down parent separator)", entry1[:8], EncodeInt64(100))
	}
	if child := page.PageID(binary.BigEndian.Uint64(entry1[8:])); child != p3 {
		t.Errorf("entry 1 child = %d, want %d (P3, right's old leftmost)", child, p3)
	}

	entry2 := left.GetEntry(2)
	if !bytes.Equal(entry2[:8], EncodeInt64(150)) {
		t.Errorf("entry 2 key = %x, want %x (150, right's original entry)", entry2[:8], EncodeInt64(150))
	}
	if child := page.PageID(binary.BigEndian.Uint64(entry2[8:])); child != p4 {
		t.Errorf("entry 2 child = %d, want %d (P4)", child, p4)
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
