// Package btree implements a persistent B+ tree on top of the page and
// storage packages.
package btree

import (
	"errors"

	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

var ErrInvalidRange = errors.New("start key must be less than end key")
var ErrDuplicateKey = errors.New("key already exists in unique index")

// BTree is a persistent B+ tree backed by a single database file.
type BTree struct {
	pm         *storage.PageManager
	rootID     page.PageID
	entrySize  uint16
	unique     bool
	pageSplits uint64
	pageMerges uint64
}

// Open opens or creates a B+ tree at path.
func Open(path string) (*BTree, error) {
	return openWith(path, 16, false)
}

// OpenUnique opens or creates a B+ tree at path with a uniqueness constraint.
// Insert fails with ErrDuplicateKey if the key already exists.
func OpenUnique(path string) (*BTree, error) {
	return openWith(path, 16, true)
}

// openWith sets up file and metadata for all BTree constructor variants.
func openWith(path string, entrySize uint16, unique bool) (*BTree, error) {
	pm, err := storage.Open(path)
	if err != nil {
		return nil, err
	}

	var rootID page.PageID
	if pm.PageCount() == 0 {
		// Brand-new file: reserve PageID 0 for metadata, so real tree
		// pages start from PageID 1.
		pm.SetNextPageID(1)
		firstLeaf := pm.AllocatePage()
		rootID = firstLeaf.ID

		metaPage := page.NewMetadataPage(0, rootID)
		if err := pm.WritePage(metaPage); err != nil {
			return nil, err
		}
		if err := pm.WritePage(firstLeaf); err != nil {
			return nil, err
		}
	} else {
		metaPage, err := pm.ReadPage(0)
		if err != nil {
			return nil, err
		}
		rootID = metaPage.RootPageID()
	}

	return &BTree{pm: pm, rootID: rootID, entrySize: entrySize, unique: unique}, nil
}

// Close closes the underlying PageManager.
func (t *BTree) Close() error {
	return t.pm.Close()
}

// Insert inserts the key-recordID pair.
func (t *BTree) Insert(key int64, recordID int64) error {
	encodedKey := EncodeInt64(key)

	leaf, ancestors, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	if t.unique {
		if _, found := findKeyIndex(leaf, encodedKey); found {
			return ErrDuplicateKey
		}
	}

	err = Insert(leaf, key, recordID)
	if err == nil {
		return t.pm.WritePage(leaf)
	}
	if err != ErrPageFull {
		return err
	}

	separatorKey, newLeaf, err := splitLeaf(leaf, t.pm.AllocatePage)
	if err != nil {
		return err
	}
	t.pageSplits++
	if err := t.updateNextLeafPrevLink(newLeaf); err != nil {
		return err
	}

	if CompareKeys(encodedKey, separatorKey) >= 0 {
		err = Insert(newLeaf, key, recordID)
	} else {
		err = Insert(leaf, key, recordID)
	}
	if err != nil {
		return err
	}

	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	if err := t.pm.WritePage(newLeaf); err != nil {
		return err
	}

	return t.propagateSplit(leaf, newLeaf, separatorKey, ancestors)
}

// Search searches for key and returns the recordID if found.
func (t *BTree) Search(key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)

	leaf, _, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return 0, false
	}

	return Search(leaf, key)
}

// ScanResult is one (key, recordID) pair returned by Scan.
type ScanResult struct {
	Key      int64
	RecordID int64
}

// Scan returns all (key, recordID) pairs with startKey <= key <= endKey.
func (t *BTree) Scan(startKey, endKey int64) ([]ScanResult, error) {
	if startKey > endKey {
		return nil, ErrInvalidRange
	}

	encodedStart := EncodeInt64(startKey)

	leaf, _, err := t.findLeafWithPath(encodedStart)
	if err != nil {
		return nil, err
	}

	var results []ScanResult

	idx, _ := findKeyIndex(leaf, encodedStart)

	for {
		for idx < leaf.NumEntries() {
			entry := leaf.GetEntry(idx)
			key := DecodeInt64(entry[:8])
			if key > endKey {
				return results, nil
			}

			results = append(results, ScanResult{
				Key:      key,
				RecordID: DecodeInt64(entry[8:]),
			})
			idx++
		}

		nextID := leaf.NextLeafPageID()
		if nextID == 0 {
			return results, nil
		}

		leaf, err = t.pm.ReadPage(nextID)
		if err != nil {
			return nil, err
		}
		idx = 0
	}
}

// Delete deletes the key from the tree.
func (t *BTree) Delete(key int64) error {
	encodedKey := EncodeInt64(key)

	leaf, ancestors, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	if err := Delete(leaf, key); err != nil {
		return err
	}

	// Root leaf has no minimum occupancy — nothing below can fire.
	if len(ancestors) == 0 {
		return t.pm.WritePage(leaf)
	}

	if leaf.NumEntries() >= page.MinEntries(t.entrySize) {
		return t.pm.WritePage(leaf)
	}

	return t.fixLeafUnderflow(leaf, ancestors)
}

// findLeafWithPath walks root-to-leaf and returns the leaf and all ancestor page IDs.
func (t *BTree) findLeafWithPath(encodedKey []byte) (leaf *page.Page, ancestors []page.PageID, err error) {
	p, err := t.pm.ReadPage(t.rootID)
	if err != nil {
		return nil, nil, err
	}

	for p.PageType() == page.InternalPage {
		ancestors = append(ancestors, p.ID)

		childID := findChildPageID(p, encodedKey)
		p, err = t.pm.ReadPage(childID)
		if err != nil {
			return nil, nil, err
		}
	}

	return p, ancestors, nil
}

// fixLeafUnderflow fixes underflowing leaf by redistribution or merge.
func (t *BTree) fixLeafUnderflow(leaf *page.Page, ancestors []page.PageID) error {
	parent, err := t.pm.ReadPage(ancestors[len(ancestors)-1])
	if err != nil {
		return err
	}

	var leftPage, rightPage *page.Page
	if leaf.PrevLeafPageID() != 0 {
		leftPage, err = t.pm.ReadPage(leaf.PrevLeafPageID())
		if err != nil {
			return err
		}
	}
	if leaf.NextLeafPageID() != 0 {
		rightPage, err = t.pm.ReadPage(leaf.NextLeafPageID())
		if err != nil {
			return err
		}
	}

	if leftPage != nil && leftPage.NumEntries() > page.MinEntries(t.entrySize) {
		return t.redistributeLeafFromLeft(leaf, leftPage, parent)
	}
	if rightPage != nil && rightPage.NumEntries() > page.MinEntries(t.entrySize) {
		return t.redistributeLeafFromRight(leaf, rightPage, parent)
	}
	if leftPage != nil {
		return t.mergeLeafWithLeft(leaf, leftPage, parent, ancestors)
	}
	return t.mergeLeafWithRight(leaf, rightPage, parent, ancestors)
}

// redistributeLeafFromLeft borrows one entry from leftPage for leaf.
func (t *BTree) redistributeLeafFromLeft(leaf, leftPage, parent *page.Page) error {
	newSeparator := redistributeFromLeft(leaf, leftPage)
	idx, _ := findChildIndex(parent, leaf.ID)
	parent.DeleteEntry(idx)
	entryBytes := append(append([]byte{}, newSeparator...), encodeChildID(leaf.ID)...)
	parent.InsertEntry(idx, entryBytes)

	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	if err := t.pm.WritePage(leftPage); err != nil {
		return err
	}
	return t.pm.WritePage(parent)
}

// redistributeLeafFromRight borrows one entry from rightPage for leaf.
func (t *BTree) redistributeLeafFromRight(leaf, rightPage, parent *page.Page) error {
	newSeparator := redistributeFromRight(leaf, rightPage)
	idx, _ := findChildIndex(parent, rightPage.ID)
	parent.DeleteEntry(idx)
	entryBytes := append(append([]byte{}, newSeparator...), encodeChildID(rightPage.ID)...)
	parent.InsertEntry(idx, entryBytes)

	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	if err := t.pm.WritePage(rightPage); err != nil {
		return err
	}
	return t.pm.WritePage(parent)
}

// mergeLeafWithLeft merges leaf into its left sibling.
func (t *BTree) mergeLeafWithLeft(leaf, leftPage, parent *page.Page, ancestors []page.PageID) error {
	mergeLeaf(leftPage, leaf)
	t.pageMerges++
	if leftPage.NextLeafPageID() != 0 {
		rightPage, err := t.pm.ReadPage(leftPage.NextLeafPageID())
		if err != nil {
			return err
		}
		rightPage.SetPrevLeafPageID(leftPage.ID)
		if err := t.pm.WritePage(rightPage); err != nil {
			return err
		}
	}
	idx, _ := findChildIndex(parent, leaf.ID)
	parent.DeleteEntry(idx)
	if err := t.pm.WritePage(leftPage); err != nil {
		return err
	}
	return t.finishInternalMerge(parent, ancestors)
}

// mergeLeafWithRight merges rightPage into leaf.
func (t *BTree) mergeLeafWithRight(leaf, rightPage, parent *page.Page, ancestors []page.PageID) error {
	mergeLeaf(leaf, rightPage)
	t.pageMerges++
	if leaf.NextLeafPageID() != 0 {
		nextPage, err := t.pm.ReadPage(leaf.NextLeafPageID())
		if err != nil {
			return err
		}
		nextPage.SetPrevLeafPageID(leaf.ID)
		if err := t.pm.WritePage(nextPage); err != nil {
			return err
		}
	}
	idx, _ := findChildIndex(parent, rightPage.ID)
	parent.DeleteEntry(idx)
	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	return t.finishInternalMerge(parent, ancestors)
}

// updateNextLeafPrevLink repairs the backward pointer of the leaf following newLeaf.
func (t *BTree) updateNextLeafPrevLink(newLeaf *page.Page) error {
	nextID := newLeaf.NextLeafPageID()
	if nextID == 0 {
		return nil
	}

	nextLeaf, err := t.pm.ReadPage(nextID)
	if err != nil {
		return err
	}
	nextLeaf.SetPrevLeafPageID(newLeaf.ID)
	return t.pm.WritePage(nextLeaf)
}

// fixInternalUnderflow fixes underflowing internal page by redistribution or merge.
func (t *BTree) fixInternalUnderflow(node *page.Page, ancestors []page.PageID) error {
	if len(ancestors) == 0 {
		if node.NumEntries() == 0 {
			childID := node.LeftmostChildPageID()
			metaPage := page.NewMetadataPage(0, childID)
			if err := t.pm.WritePage(metaPage); err != nil {
				return err
			}
			t.rootID = childID
			return nil
		}
		return t.pm.WritePage(node)
	}

	grandparent, err := t.pm.ReadPage(ancestors[len(ancestors)-1])
	if err != nil {
		return err
	}

	leftID, rightID := findSiblingPageIDs(grandparent, node)

	var leftSibling, rightSibling *page.Page
	if leftID != 0 {
		leftSibling, err = t.pm.ReadPage(leftID)
		if err != nil {
			return err
		}
	}
	if rightID != 0 {
		rightSibling, err = t.pm.ReadPage(rightID)
		if err != nil {
			return err
		}
	}

	if leftSibling != nil && leftSibling.NumEntries() > page.MinEntries(t.entrySize) {
		return t.redistributeInternalNodeFromLeft(node, leftSibling, grandparent)
	}
	if rightSibling != nil && rightSibling.NumEntries() > page.MinEntries(t.entrySize) {
		return t.redistributeInternalNodeFromRight(node, rightSibling, grandparent)
	}
	if leftSibling != nil {
		return t.mergeInternalWithLeft(node, leftSibling, grandparent, ancestors)
	}
	return t.mergeInternalWithRight(node, rightSibling, grandparent, ancestors)
}

// redistributeInternalNodeFromLeft borrows one entry from leftSibling for node.
func (t *BTree) redistributeInternalNodeFromLeft(node, leftSibling, grandparent *page.Page) error {
	idx, _ := findChildIndex(grandparent, node.ID)
	oldSeparator := grandparent.GetEntry(idx)[:8]

	newSeparator := redistributeInternalFromLeft(node, leftSibling, oldSeparator)

	grandparent.DeleteEntry(idx)
	entryBytes := append(append([]byte{}, newSeparator...), encodeChildID(node.ID)...)
	grandparent.InsertEntry(idx, entryBytes)

	if err := t.pm.WritePage(node); err != nil {
		return err
	}
	if err := t.pm.WritePage(leftSibling); err != nil {
		return err
	}
	return t.pm.WritePage(grandparent)
}

// redistributeInternalNodeFromRight borrows one entry from rightSibling for node.
func (t *BTree) redistributeInternalNodeFromRight(node, rightSibling, grandparent *page.Page) error {
	idx, _ := findChildIndex(grandparent, rightSibling.ID)
	oldSeparator := grandparent.GetEntry(idx)[:8]

	newSeparator := redistributeInternalFromRight(node, rightSibling, oldSeparator)

	grandparent.DeleteEntry(idx)
	entryBytes := append(append([]byte{}, newSeparator...), encodeChildID(rightSibling.ID)...)
	grandparent.InsertEntry(idx, entryBytes)

	if err := t.pm.WritePage(node); err != nil {
		return err
	}
	if err := t.pm.WritePage(rightSibling); err != nil {
		return err
	}
	return t.pm.WritePage(grandparent)
}

// mergeInternalWithLeft merges node into its left sibling.
func (t *BTree) mergeInternalWithLeft(node, leftSibling, grandparent *page.Page, ancestors []page.PageID) error {
	idx, _ := findChildIndex(grandparent, node.ID)
	separator := grandparent.GetEntry(idx)[:8]

	mergeInternal(leftSibling, node, separator)
	t.pageMerges++
	grandparent.DeleteEntry(idx)

	if err := t.pm.WritePage(leftSibling); err != nil {
		return err
	}

	return t.finishInternalMerge(grandparent, ancestors)
}

// mergeInternalWithRight merges rightSibling into node.
func (t *BTree) mergeInternalWithRight(node, rightSibling, grandparent *page.Page, ancestors []page.PageID) error {
	idx, _ := findChildIndex(grandparent, rightSibling.ID)
	separator := grandparent.GetEntry(idx)[:8]

	mergeInternal(node, rightSibling, separator)
	t.pageMerges++
	grandparent.DeleteEntry(idx)

	if err := t.pm.WritePage(node); err != nil {
		return err
	}

	return t.finishInternalMerge(grandparent, ancestors)
}

// finishInternalMerge checks grandparent for underflow after merge.
func (t *BTree) finishInternalMerge(grandparent *page.Page, ancestors []page.PageID) error {
	isRoot := len(ancestors) == 1 // grandparent's OWN ancestors would be empty
	if !isRoot && grandparent.NumEntries() >= page.MinEntries(t.entrySize) {
		return t.pm.WritePage(grandparent)
	}
	if isRoot && grandparent.NumEntries() > 0 {
		// Root, but not shrunk to a single child — no minimum applies.
		return t.pm.WritePage(grandparent)
	}
	return t.fixInternalUnderflow(grandparent, ancestors[:len(ancestors)-1])
}

// propagateSplit wires a completed split into the tree structure.
func (t *BTree) propagateSplit(p, newPage *page.Page, separatorKey []byte, ancestors []page.PageID) error {
	if len(ancestors) == 0 {
		newRootPage := t.pm.AllocatePage()
		newRoot := page.NewInternalPage(newRootPage.ID, p.ID)
		entryBytes := append(append([]byte{}, separatorKey...), encodeChildID(newPage.ID)...)
		newRoot.InsertEntry(0, entryBytes)
		if err := t.pm.WritePage(newRoot); err != nil {
			return err
		}

		// The root changed — update the durable metadata page (PageID
		// 0) so a future Open recovers the real root instead of the
		// stale one.
		metaPage := page.NewMetadataPage(0, newRoot.ID)
		if err := t.pm.WritePage(metaPage); err != nil {
			return err
		}

		t.rootID = newRoot.ID
		return nil
	}

	parentID := ancestors[len(ancestors)-1]
	remaining := ancestors[:len(ancestors)-1]

	parent, err := t.pm.ReadPage(parentID)
	if err != nil {
		return err
	}

	entryBytes := append(append([]byte{}, separatorKey...), encodeChildID(newPage.ID)...)

	if parent.HasSpaceFor(len(entryBytes)) {
		idx, _ := findKeyIndex(parent, separatorKey)
		parent.InsertEntry(idx, entryBytes)
		return t.pm.WritePage(parent)
	}

	var parentSeparator []byte
	var newParent *page.Page
	if parent.PageType() == page.InternalPage {
		parentSeparator, newParent, err = splitInternal(parent, t.pm.AllocatePage)
	} else {
		parentSeparator, newParent, err = splitLeaf(parent, t.pm.AllocatePage)
		if err == nil {
			err = t.updateNextLeafPrevLink(newParent)
		}
	}
	if err != nil {
		return err
	}
	t.pageSplits++

	// Decide which half of the split parent should receive
	// (separatorKey, newPage.ID), same idea as the leaf case in Insert.
	if CompareKeys(separatorKey, parentSeparator) >= 0 {
		idx, _ := findKeyIndex(newParent, separatorKey)
		newParent.InsertEntry(idx, entryBytes)
	} else {
		idx, _ := findKeyIndex(parent, separatorKey)
		parent.InsertEntry(idx, entryBytes)
	}

	if err := t.pm.WritePage(parent); err != nil {
		return err
	}
	if err := t.pm.WritePage(newParent); err != nil {
		return err
	}

	return t.propagateSplit(parent, newParent, parentSeparator, remaining)
}
