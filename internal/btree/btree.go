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
	p, err := t.pm.ReadPage(t.rootID)
	if err != nil {
		return nil, err
	}

	for p.PageType() == page.InternalPage {
		childID := findChildPageID(p, encodedKey)
		p, err = t.pm.ReadPage(childID)
		if err != nil {
			return nil, err
		}
	}

	return p, nil
}

// Insert adds a (key, recordID) pair to the tree.
func (t *BTree) Insert(key int64, recordID int64) error {
	encodedKey := EncodeInt64(key)

	leaf, err := t.findLeaf(encodedKey)
	if err != nil {
		return err
	}

	err = Insert(leaf, key, recordID)
	if err == nil {
		// fit without splitting — just persist and we're done
		return t.pm.WritePage(leaf)
	}
	if err != ErrPageFull {
		return err
	}

	// leaf is full: split it, then handle promoting the separator.
	return t.insertWithSplit(leaf, key, recordID)
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

// insertWithSplit handles the case where leaf was full: split it, insert
// the new key into whichever resulting half it belongs in, and wire the
// split into the tree structure.
//
// SCOPE FOR TONIGHT: only handles the case where the split leaf's parent
// is nothing yet (leaf was the root). This covers going from a
// single-leaf tree to a root+2-leaves tree. It does NOT yet handle
// splitting a leaf that already has an internal parent — that's a real
// gap, to close next session. Given tonight's leaf always starts as the
// sole root, this gap isn't reachable yet.
func (t *BTree) insertWithSplit(leaf *page.Page, key, recordID int64) error {
	separatorKey, newLeaf, err := splitLeaf(leaf, t.pm.AllocatePage)
	if err != nil {
		return err
	}

	encodedKey := EncodeInt64(key)
	if CompareKeys(encodedKey, separatorKey) >= 0 {
		if err := Insert(newLeaf, key, recordID); err != nil {
			return err
		}
	} else {
		if err := Insert(leaf, key, recordID); err != nil {
			return err
		}
	}

	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	if err := t.pm.WritePage(newLeaf); err != nil {
		return err
	}

	// Create a new root: an internal page whose leftmost child is the
	// OLD leaf (still holding the smaller keys, still at its original
	// PageID), and whose one entry is (separatorKey, newLeaf.ID).
	newRootPage := t.pm.AllocatePage()
	newRoot := page.NewInternalPage(newRootPage.ID, leaf.ID)

	entryBytes := append(append([]byte{}, separatorKey...), encodeChildID(newLeaf.ID)...)
	newRoot.InsertEntry(0, entryBytes)

	if err := t.pm.WritePage(newRoot); err != nil {
		return err
	}

	// The new internal page is now the root.
	t.rootID = newRoot.ID

	return nil

}
