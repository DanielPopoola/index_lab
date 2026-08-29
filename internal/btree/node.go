package btree

import (
	"encoding/binary"
	"sort"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// encodeChildID encodes a PageID as 8 big-endian bytes.
func encodeChildID(id page.PageID) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

// entryKeyLen returns the key portion length of entries in page p.
func entryKeyLen(p *page.Page) int {
	if p == nil || p.NumEntries() == 0 {
		return 8
	}
	return len(p.GetEntry(0)) - 8
}

// findChildPageID returns the child PageID a search for targetKey should descend into.
func findChildPageID(p *page.Page, targetKey []byte) page.PageID {
	n := p.NumEntries()
	if n == 0 {
		return p.LeftmostChildPageID()
	}
	keyLen := entryKeyLen(p)
	idx := sort.Search(int(n), func(i int) bool {
		return CompareKeys(p.GetEntry(uint16(i))[:keyLen], targetKey) == 1
	})

	if idx == 0 {
		return p.LeftmostChildPageID()
	}

	entry := p.GetEntry(uint16(idx - 1))
	childBytes := entry[keyLen:]
	childID := binary.BigEndian.Uint64(childBytes)
	return page.PageID(childID)
}

// findKeyIndex does binary search over p's entries for targetKey.
func findKeyIndex(p *page.Page, targetKey []byte) (index uint16, found bool) {
	n := p.NumEntries()
	if n == 0 {
		return 0, false
	}
	keyLen := entryKeyLen(p)

	i := sort.Search(int(n), func(i int) bool {
		return CompareKeys(p.GetEntry(uint16(i))[:keyLen], targetKey) >= 0
	})

	if i < int(n) && CompareKeys(p.GetEntry(uint16(i))[:keyLen], targetKey) == 0 {
		return uint16(i), true
	}

	return uint16(i), false
}

// findChildIndex finds the entry index in internal page p with a child pointer matching childID.
// If childID is the leftmost child, returns (p.NumEntries(), true) as a sentinel.
func findChildIndex(p *page.Page, childID page.PageID) (index uint16, found bool) {
	if p.LeftmostChildPageID() == childID {
		return p.NumEntries(), true
	}
	keyLen := entryKeyLen(p)

	for i := uint16(0); i < p.NumEntries(); i++ {
		childBytes := p.GetEntry(i)[keyLen:]
		pageID := page.PageID(binary.BigEndian.Uint64(childBytes))
		if pageID == childID {
			return i, true
		}
	}
	return 0, false
}

// findSiblingPageIDs finds node's left and right sibling PageIDs among grandparent's children.
func findSiblingPageIDs(grandparent, node *page.Page) (leftID, rightID page.PageID) {
	idx, _ := findChildIndex(grandparent, node.ID)
	n := grandparent.NumEntries()
	keyLen := entryKeyLen(grandparent)
	if idx == n {
		return 0, page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(0)[keyLen:]))
	}

	if idx == 0 {
		leftID = grandparent.LeftmostChildPageID()
	} else {
		leftID = page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(idx - 1)[keyLen:]))
	}

	if idx+1 < n {
		rightID = page.PageID(binary.BigEndian.Uint64(grandparent.GetEntry(idx + 1)[keyLen:]))
	}
	return leftID, rightID
}

// redistributeFromLeft borrows one entry from leftSibling to fix underflowing's occupancy.
func redistributeFromLeft(underflowing, leftSibling *page.Page) (newSeparator []byte) {
	lastIdx := leftSibling.NumEntries() - 1
	entry := append([]byte(nil), leftSibling.GetEntry(lastIdx)...)
	keyLen := len(entry) - 8

	leftSibling.DeleteEntry(lastIdx)

	underflowing.InsertEntry(0, entry)

	return entry[:keyLen]
}

// redistributeFromRight borrows one entry from rightSibling to fix underflowing's occupancy.
func redistributeFromRight(underflowing, rightSibling *page.Page) (newSeparator []byte) {
	entry := append([]byte(nil), rightSibling.GetEntry(0)...)
	keyLen := len(entry) - 8

	rightSibling.DeleteEntry(0)

	underflowing.InsertEntry(underflowing.NumEntries(), entry)

	return rightSibling.GetEntry(0)[:keyLen]
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

// mergeInternal merges right into left with the old parent separator.
func mergeInternal(left, right *page.Page, oldParentSeparator []byte) {
	leftmostChild := right.LeftmostChildPageID()
	entryBytes := append(append([]byte{}, oldParentSeparator...), encodeChildID(leftmostChild)...)

	left.InsertEntry(left.NumEntries(), entryBytes)
	for i := uint16(0); i < right.NumEntries(); i++ {
		left.InsertEntry(left.NumEntries(), right.GetEntry(i))
	}
}

// redistributeInternalFromLeft borrows one entry from leftSibling for underflowing.
func redistributeInternalFromLeft(underflowing, leftSibling *page.Page, oldParentSeparator []byte) (newParentSeparator []byte) {
	lastIdx := leftSibling.NumEntries() - 1
	lastEntry := append([]byte(nil), leftSibling.GetEntry(lastIdx)...)
	movedChildBytes := lastEntry[8:]
	newParentSeparator = lastEntry[:len(lastEntry)-8]

	oldLeftmostChild := underflowing.LeftmostChildPageID()

	leftSibling.DeleteEntry(lastIdx)

	newFirstEntry := append(append([]byte{}, oldParentSeparator...), encodeChildID(oldLeftmostChild)...)
	underflowing.InsertEntry(0, newFirstEntry)
	underflowing.SetLeftmostChildPageID(page.PageID(binary.BigEndian.Uint64(movedChildBytes)))
	return newParentSeparator
}

// redistributeInternalFromRight borrows one entry from rightSibling for underflowing.
func redistributeInternalFromRight(underflowing, rightSibling *page.Page, oldParentSeparator []byte) (newParentSeparator []byte) {
	oldLeftmostChild := rightSibling.LeftmostChildPageID()
	firstEntry := append([]byte(nil), rightSibling.GetEntry(0)...)
	oldFirstEntry := firstEntry[8:]
	newParentSeparator = firstEntry[:len(firstEntry)-8]

	rightSibling.DeleteEntry(0)

	latestEntry := append(append([]byte{}, oldParentSeparator...), encodeChildID(oldLeftmostChild)...)
	underflowing.InsertEntry(underflowing.NumEntries(), latestEntry)
	rightSibling.SetLeftmostChildPageID(page.PageID(binary.BigEndian.Uint64(oldFirstEntry)))
	return newParentSeparator
}

// splitLeaf splits a full leaf page into two.
func splitLeaf(oldPage *page.Page, allocateFn func() *page.Page) (separatorKey []byte, newPage *page.Page, err error) {
	mid := oldPage.NumEntries() / 2
	separatorLen := len(oldPage.GetEntry(0)) - 8

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

	separatorKey = newPage.GetEntry(0)[:separatorLen]

	return separatorKey, newPage, nil
}

// splitInternal splits a full internal page into two.
func splitInternal(oldPage *page.Page, allocateFn func() *page.Page) (separatorKey []byte, newPage *page.Page, err error) {
	mid := oldPage.NumEntries() / 2
	separatorLen := entryKeyLen(oldPage)

	middleEntry := append([]byte(nil), oldPage.GetEntry(mid)...)
	separatorKey = middleEntry[:separatorLen]
	childBytes := middleEntry[separatorLen:]
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
