// Package btree implements a persistent B+ tree on top of the page and
// storage packages. It ties together storage.PageManager (disk I/O)
// with the leaf-level Insert/Search from tree.go, adding: root
// tracking, multi-level root-to-leaf traversal, and insertion with
// splitting (leaf splits, internal-page splits, and root splits, all
// propagated via propagateSplit).
//
// CURRENT SIMPLIFICATION: rootID is tracked only in memory and does not
// survive reopening a tree whose root has ever changed (e.g. after a
// split grew the tree past one level) — Open still assumes rootID is
// PageID 0 on reopen. A persisted, changeable root ID (via a dedicated
// metadata page) is needed to fix this; see the deferred work noted in
// project memory.
package btree

import (
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

// BTree is a persistent B+ tree backed by a single database file. It
// wraps a storage.PageManager and adds tree structure on top: root
// tracking, multi-level traversal, and insertion with splitting.
type BTree struct {
	pm     *storage.PageManager
	rootID page.PageID
}

// Open opens (or creates) a B+ tree backed by the database file at
// path. On reopen, rootID is currently always assumed to be PageID 0 —
// see the package-level CURRENT SIMPLIFICATION note. This is correct as
// long as the root has never split; once persisted root-ID metadata
// exists, reopening a tree whose root changed will need to read the
// real root ID instead of assuming 0.
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
// an actual leaf page ready for Insert/Search. It is findLeafWithPath
// with the ancestor path discarded, for callers (like Search) that only
// need the leaf.
func (t *BTree) findLeaf(encodedKey []byte) (*page.Page, error) {
	leaf, _, err := t.findLeafWithPath(encodedKey)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}

// findLeafWithPath walks down from the root exactly like findLeaf, but
// additionally records the PageID of every internal page visited along
// the way, in top-down order (root first, immediate parent last). This
// gives the caller a bottom-up path to walk back up if the leaf ends up
// needing to split: pop the last-visited (closest) ancestor first. Only
// internal pages are ever pushed — the loop that pushes exits before
// processing a leaf, so a leaf's own ID never lands on its own
// ancestor stack.
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

// Insert adds a (key, recordID) pair to the tree, splitting leaf and
// internal pages as needed and growing the tree's height if the root
// itself ends up splitting.
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

// Search looks up key in the tree and reports whether it was found.
func (t *BTree) Search(key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)

	leaf, err := t.findLeaf(encodedKey)
	if err != nil {
		return 0, false
	}

	return Search(leaf, key)
}

// propagateSplit wires an already-completed split (p, newPage,
// separatorKey) into the tree. p holds the smaller half of whatever
// just split and newPage the larger half; propagateSplit does not
// split p itself (see Insert, which splits the leaf before calling
// this). It pops the closest ancestor off ancestors and tries to
// insert (separatorKey, newPage.ID) there:
//
//   - if ancestors is empty, p had no parent (it was the root), so a
//     new root page is allocated with p as its leftmost child.
//   - if the popped parent has space, the entry is inserted directly.
//   - if the parent is also full, the parent itself is split (leaf or
//     internal, via splitLeaf/splitInternal as appropriate), the
//     pending entry is placed in whichever half it belongs to, and
//     propagateSplit recurses one level further up with the remaining
//     ancestors. Recursion depth is bounded by tree height, which
//     stays small by construction given B+ tree fan-out.
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
