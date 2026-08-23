// BTree ties together storage.PageManager (disk I/O) and the leaf-level
// Insert/Search, adding: knowing which page is root, and now, walking
// down through however many internal-page levels exist to reach the
// correct leaf.
//
// CURRENT SIMPLIFICATION: rootID is assumed to always be PageID 0. This
// holds until root-splitting is implemented, at which point a persisted,
// changeable root ID (via a dedicated metadata page) will be needed.
package btree

import (
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

type BTree struct {
	pm     *storage.PageManager
	rootID page.PageID
}

// Open opens (or creates) a B+ tree backed by the database file at path.
func Open(path string) (*BTree, error) {
	pm, err := storage.Open(path)
	if err != nil {
		return nil, err
	}

	var rootID page.PageID
	if pm.PageCount() == 0 {
		root := pm.AllocatePage()
		rootID = root.ID
		if err := pm.WritePage(root); err != nil {
			return nil, err
		}
	} else {
		rootID = 0
	}

	return &BTree{pm: pm, rootID: rootID}, nil
}

// Close closes the underlying PageManager.
func (t *BTree) Close() error {
	return t.pm.Close()
}

// findLeaf walks down from the root, following findChildPageID through
// however many internal-page levels exist, until it reaches and returns
// an actual LEAF page ready for Insert/Search.
func (t *BTree) findLeaf(encodedKey []byte) (*page.Page, error) {
	leaf, _, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}

// findLeafWithPath walks down from the root exactly like findLeaf, but
// additionally records the PageID of every internal page visited along
// the way, in top-down order (root first). This gives the caller a
// bottom-up path to walk back up if the leaf ends up needing to split:
// pop the last-visited (closest) ancestor first.
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

// Insert adds a (key, recordID) pair to the tree.
func (t *BTree) Insert(key int64, recordID int64) error {
	encodedKey := EncodeInt64(key)

	leaf, ancestors, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	err = Insert(leaf, key, recordID)
	if err == nil {
		return t.pm.WritePage(leaf)
	}
	if err != ErrPageFull {
		return err
	}

	// Leaf is full. Split it, insert the new key into whichever half
	// it belongs in, then let propagateSplit wire the split into the
	// tree (parent insert, recursive split, or new root).
	separatorKey, newLeaf, err := splitLeaf(leaf, t.pm.AllocatePage)
	if err != nil {
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

// Search looks up a key in the tree.
func (t *BTree) Search(key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)

	leaf, err := t.findLeaf(encodedKey)
	if err != nil {
		return 0, false
	}

	return Search(leaf, key)
}

// propagateSplit wires an already-completed split (p, newPage,
// separatorKey) into the tree: push the separator into the parent
// (from ancestors), or make a new root if there's no parent. If the
// parent itself is full, split the parent too, and recurse.
func (t *BTree) propagateSplit(p, newPage *page.Page, separatorKey []byte, ancestors []page.PageID) error {
	if len(ancestors) == 0 {
		newRootPage := t.pm.AllocatePage()
		newRoot := page.NewInternalPage(newRootPage.ID, p.ID)
		entryBytes := append(append([]byte{}, separatorKey...), encodeChildID(newPage.ID)...)
		newRoot.InsertEntry(0, entryBytes)
		if err := t.pm.WritePage(newRoot); err != nil {
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
		idx, _ := findInsertIndex(parent, separatorKey)
		parent.InsertEntry(idx, entryBytes)
		return t.pm.WritePage(parent)
	}

	var parentSeparator []byte
	var newParent *page.Page
	if parent.PageType() == page.InternalPage {
		parentSeparator, newParent, err = splitInternal(parent, t.pm.AllocatePage)
	} else {
		parentSeparator, newParent, err = splitLeaf(parent, t.pm.AllocatePage)
	}
	if err != nil {
		return err
	}

	// Decide which half of the split parent should receive
	// (separatorKey, newPage.ID), same idea as the leaf case in Insert.
	if CompareKeys(separatorKey, parentSeparator) >= 0 {
		idx, _ := findInsertIndex(newParent, separatorKey)
		newParent.InsertEntry(idx, entryBytes)
	} else {
		idx, _ := findInsertIndex(parent, separatorKey)
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
