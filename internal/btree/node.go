package btree

import (
	"encoding/binary"
	"sort"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// Encodes `id` as 8 big-endian bytes, for use as the value half of an internal-page entry.
func encodeChildID(id page.PageID) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

// For an internal page `p`, returns the child `PageID` a search for `targetKey` should descend into.
func findChildPageID(p *page.Page, targetKey []byte) page.PageID {
	n := p.NumEntries()
	idx := sort.Search(int(n), func(i int) bool {
		return CompareKeys(p.GetEntry(uint16(i))[:8], targetKey) == 1
	})

	if idx == 0 {
		return p.LeftmostChildPageID()
	}

	entry := p.GetEntry(uint16(idx - 1))
	childBytes := entry[8:]
	childID := binary.BigEndian.Uint64(childBytes)
	return page.PageID(childID)
}

// Binary search over `p`'s sorted entries for `targetKey`, comparing the first 8 bytes of each entry.
// Works on both leaf and internal pages (both store the key as the first 8 bytes of every entry).
func findKeyIndex(p *page.Page, targetKey []byte) (index uint16, found bool) {
	n := p.NumEntries()

	i := sort.Search(int(n), func(i int) bool {
		return CompareKeys(p.GetEntry(uint16(i))[:8], targetKey) >= 0
	})

	if i < int(n) && CompareKeys(p.GetEntry(uint16(i))[:8], targetKey) == 0 {
		return uint16(i), true
	}

	return uint16(i), false
}

// Finds which entry index in internal page `p` has a child pointer matching
// `childID`. Used after a merge, to find and remove the dead child's
// entry from its parent.
//
// If `childID` matches p.LeftmostChildPageID() instead of any entry
// (the leftmost child isn't stored in the entry array at all), returns
// (p.NumEntries(), true) — a sentinel that can never collide with a
// real entry index, since valid indices only go up to NumEntries()-1.
// Callers must check for this case separately; it can't be removed
// with DeleteEntry the way a normal entry can.
func findChildIndex(p *page.Page, childID page.PageID) (index uint16, found bool) {
	if p.LeftmostChildPageID() == childID {
		return p.NumEntries(), true
	}

	for i := uint16(0); i < p.NumEntries(); i++ {
		childBytes := p.GetEntry(i)[8:]
		pageID := page.PageID(binary.BigEndian.Uint64(childBytes))
		if pageID == childID {
			return i, true
		}
	}
	return 0, false
}

// Finds `node`'s left and right sibling PageIDs among `grandparent`'s
// children. Internal pages don't carry stored Prev/Next pointers like
// leaves do, so a node's siblings can only be found by locating its
// position among its own parent's children and looking one position
// to either side.
//
// Applies the mapping documented on findChildIndex. Returns 0 for
// whichever side doesn't exist (node is the first or last child).
func findSiblingPageIDs(grandparent, node *page.Page) (leftID, rightID page.PageID) {
	idx, _ := findChildIndex(grandparent, node.ID)
	n := grandparent.NumEntries()
	if idx == n {
		return 0, page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(0)[8:]))
	}

	if idx == 0 {
		leftID = grandparent.LeftmostChildPageID()
	} else {
		leftID = page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(idx - 1)[8:]))
	}

	if idx+1 < n {
		rightID = page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(idx + 1)[8:]))
	}
	return leftID, rightID
}

// Borrows exactly one entry from `leftSibling` to fix `underflowing`'s
// occupancy. Moves `leftSibling`'s LAST (largest-key) entry into
// `underflowing` — the only entry that can move while keeping both
// pages sorted and keeping every key in `leftSibling` still smaller
// than every key in `underflowing`.
func redistributeFromLeft(underflowing, leftSibling *page.Page) (newSeparator []byte) {
	lastIdx := leftSibling.NumEntries() - 1
	// DeleteEntry compacts leftSibling, so copy the entry before deleting it.
	entry := append([]byte(nil), leftSibling.GetEntry(lastIdx)...)

	leftSibling.DeleteEntry(lastIdx)

	underflowing.InsertEntry(0, entry)

	return entry[:8]
}

// Borrows exactly one entry from `rightSibling` to fix `underflowing`'s
// occupancy. Moves `rightSibling`'s FIRST (smallest-key) entry into
// `underflowing` — the only entry that can move while keeping both
// pages sorted and keeping every key in `rightSibling` still larger
// than every key in `underflowing`.
func redistributeFromRight(underflowing, rightSibling *page.Page) (newSeparator []byte) {
	// DeleteEntry compacts rightSibling, so copy the entry before deleting it.
	entry := append([]byte(nil), rightSibling.GetEntry(0)...)

	rightSibling.DeleteEntry(0)

	underflowing.InsertEntry(underflowing.NumEntries(), entry)

	return rightSibling.GetEntry(0)[:8]
}

// Merges `right` into `left`. `left` must hold smaller keys than
// `right` (i.e. left is right's PrevLeafPageID in the tree's leaf
// chain). After this call, `left` holds every entry from both pages;
// `right` is empty and its page is the caller's responsibility to
// remove from the tree (delete its parent entry, free its PageID).
func mergeLeaf(left, right *page.Page) {
	for i := uint16(0); i < right.NumEntries(); i++ {
		left.InsertEntry(left.NumEntries(), right.GetEntry(i))
	}

	left.SetNextLeafPageID(right.NextLeafPageID())
	for i := int(right.NumEntries()) - 1; i >= 0; i-- {
		right.DeleteEntry(uint16(i))
	}
}

func mergeInternal(left, right *page.Page, oldParentSeparator []byte) {
	leftmostChild := right.LeftmostChildPageID()
	entryBytes := append(append([]byte{}, oldParentSeparator...), encodeChildID(leftmostChild)...)

	left.InsertEntry(left.NumEntries(), entryBytes)
	for i := uint16(0); i < right.NumEntries(); i++ {
		left.InsertEntry(left.NumEntries(), right.GetEntry(i))
	}
}

func redistributeInternalFromLeft(underflowing, leftSibling *page.Page, oldParentSeparator []byte) (newParentSeparator []byte) {
	lastIdx := leftSibling.NumEntries() - 1
	lastEntry := append([]byte(nil), leftSibling.GetEntry(lastIdx)...)
	movedChildBytes := lastEntry[8:]
	newParentSeparator = lastEntry[:8]

	oldLeftmostChild := underflowing.LeftmostChildPageID()

	leftSibling.DeleteEntry(lastIdx)

	newFirstEntry := append(append([]byte{}, oldParentSeparator...), encodeChildID(oldLeftmostChild)...)
	underflowing.InsertEntry(0, newFirstEntry)
	underflowing.SetLeftmostChildPageID(page.PageID(binary.BigEndian.Uint64(movedChildBytes)))
	return newParentSeparator
}

func redistributeInternalFromRight(underflowing, rightSibling *page.Page, oldParentSeparator []byte) (newParentSeparator []byte) {
	oldLeftmostChild := rightSibling.LeftmostChildPageID()
	firstEntry := append([]byte(nil), rightSibling.GetEntry(0)...)
	oldFirstEntry := firstEntry[8:]
	newParentSeparator = firstEntry[:8]

	rightSibling.DeleteEntry(0)

	latestEntry := append(append([]byte{}, oldParentSeparator...), encodeChildID(oldLeftmostChild)...)
	underflowing.InsertEntry(underflowing.NumEntries(), latestEntry)
	rightSibling.SetLeftmostChildPageID(page.PageID(binary.BigEndian.Uint64(oldFirstEntry)))
	return newParentSeparator
}

// Splits a full leaf page. The smaller half (lower `NumEntries()/2` entries) stays in `oldPage`; the larger half moves to a newly allocated page.
// Sibling pointers (`NextLeafPageID`/`PrevLeafPageID`) are rewired to keep the leaf linked list correct.
// `separatorKey` is the new page's first key — a real, retrievable entry (leaf keys are never discarded).
func splitLeaf(oldPage *page.Page, allocateFn func() *page.Page) (separatorKey []byte, newPage *page.Page, err error) {
	mid := oldPage.NumEntries() / 2

	var moving [][]byte
	for i := mid; i < oldPage.NumEntries(); i++ {
		moving = append(moving, append([]byte(nil), oldPage.GetEntry(i)...))
	}

	for i := oldPage.NumEntries() - 1; i >= mid; i-- {
		oldPage.DeleteEntry(i)
	}

	newPage = allocateFn()
	for _, entryBytes := range moving {
		newPage.InsertEntry(newPage.NumEntries(), entryBytes)
	}
	newPage.SetPrevLeafPageID(oldPage.ID)
	newPage.SetNextLeafPageID(oldPage.NextLeafPageID())
	oldPage.SetNextLeafPageID(newPage.ID)

	separatorKey = newPage.GetEntry(0)[:8]

	return separatorKey, newPage, nil
}

// Splits a full internal page in three parts: entries before the midpoint stay in `oldPage`; entries after it move to a newly allocated page;
// the midpoint entry is consumed entirely — its key is promoted out as `separatorKey` (not retained in either half), and its child pointer becomes `newPage`'s leftmost child
func splitInternal(oldPage *page.Page, allocateFn func() *page.Page) (separatorKey []byte, newPage *page.Page, err error) {
	mid := oldPage.NumEntries() / 2

	middleEntry := append([]byte(nil), oldPage.GetEntry(mid)...)
	separatorKey = middleEntry[:8]
	childBytes := middleEntry[8:]
	middleChildID := page.PageID(binary.BigEndian.Uint64(childBytes))

	var moving [][]byte
	for i := mid + 1; i < oldPage.NumEntries(); i++ {
		moving = append(moving, append([]byte(nil), oldPage.GetEntry(i)...))
	}

	reserved := allocateFn()
	newPage = page.NewInternalPage(reserved.ID, middleChildID)

	for _, entryBytes := range moving {
		newPage.InsertEntry(newPage.NumEntries(), entryBytes)
	}

	for i := oldPage.NumEntries() - 1; i >= mid; i-- {
		oldPage.DeleteEntry(i)
	}

	return separatorKey, newPage, nil
}
