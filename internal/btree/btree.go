// Package btree implements a persistent B+ tree on top of the page and
// storage packages.
package btree

import (
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

// Persistent B+ tree backed by a single database file.
// Wraps a `*storage.PageManager` and tracks the current root `PageID`.
type BTree struct {
	pm     *storage.PageManager
	rootID page.PageID
}

// Opens or creates a B+ tree at `path`.
func Open(path string) (*BTree, error) {
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

	return &BTree{pm: pm, rootID: rootID}, nil
}

// Closes the underlying `PageManager`.
func (t *BTree) Close() error {
	return t.pm.Close()
}

// Inserts `(key, recordID)`.
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

// Looks up `key`
func (t *BTree) Search(key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)

	leaf, _, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return 0, false
	}

	return Search(leaf, key)
}

// Deletes `key`.
func (t *BTree) Delete(key int64) error {
	encodedKey := EncodeInt64(key)

	leaf, ancestors, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	if err := Delete(leaf, key); err != nil {
		return err
	}

	if len(ancestors) == 0 {
		return t.pm.WritePage(leaf)
	}

	if leaf.NumEntries() >= page.MinEntries() {
		return t.pm.WritePage(leaf)
	}

	return t.fixLeafUnderflow(leaf, ancestors)
}

// Walks root-to-leaf, following child pointers through however many internal-page levels exist., additionally returning `ancestors`:
// every internal page visited on the way down, in top-down order (root first, immediate parent of the leaf last).
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

// Called when `leaf` has dropped below MinEntries() after a delete and
// is not the root. Loads whichever siblings exist, then decides:
// redistribute (cheaper, tried first) or merge (fallback when no
// sibling has spare capacity).
//
// ancestors is the top-down path from findLeafWithPath; the immediate
// parent is ancestors[len(ancestors)-1].
//
// For now this stops the cascade at the parent: if removing the dead
// child's entry from the parent leaves the PARENT underflowing too,
// this function still writes the parent as-is and returns — the
// parent-level cascade is the next layer to add on top of this one.
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

	if leftPage != nil && leftPage.NumEntries() > page.MinEntries() {
		return t.redistributeLeafFromLeft(leaf, leftPage, parent)
	}
	if rightPage != nil && rightPage.NumEntries() > page.MinEntries() {
		return t.redistributeLeafFromRight(leaf, rightPage, parent)
	}
	if leftPage != nil {
		return t.mergeLeafWithLeft(leaf, leftPage, parent)
	}
	return t.mergeLeafWithRight(leaf, rightPage, parent)
}

// Borrows one entry from leaf's left sibling and updates the parent's
// separator entry for leaf accordingly.
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

// Borrows one entry from leaf's right sibling and updates the parent's
// separator entry for rightPage accordingly (not leaf — see the
// asymmetry noted on redistributeFromRight).
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

// Merges leaf into its left sibling. leftPage survives (absorbs leaf's
// entries); leaf is discarded. Patches the third page (whatever now
// sits after leftPage in the chain) if one exists, then removes leaf's
// now-meaningless entry from the parent.
func (t *BTree) mergeLeafWithLeft(leaf, leftPage, parent *page.Page) error {
	mergeLeaf(leftPage, leaf)
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
	return t.pm.WritePage(parent)
}

func (t *BTree) mergeLeafWithRight(leaf, rightPage, parent *page.Page) error {
	mergeLeaf(leaf, rightPage)
	if leaf.NextLeafPageID() != 0 {
		extraPage, err := t.pm.ReadPage(leaf.NextLeafPageID())
		if err != nil {
			return err
		}
		extraPage.SetPrevLeafPageID(leaf.ID)
		if err := t.pm.WritePage(extraPage); err != nil {
			return err
		}
	}
	idx, _ := findChildIndex(parent, rightPage.ID)
	parent.DeleteEntry(idx)
	if err := t.pm.WritePage(leaf); err != nil {
		return err
	}
	return t.pm.WritePage(parent)
}

// Wires an already-completed split into the tree structure.
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
	}
	if err != nil {
		return err
	}

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
