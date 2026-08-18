package btree

import (
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

type BTree struct {
	pm     *storage.PageManager
	rootID page.PageID
}

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

func (t *BTree) Insert(key, recordID int64) error {
	p, err := t.pm.ReadPage(t.rootID)
	if err != nil {
		return err
	}

	if err := Insert(p, key, recordID); err != nil {
		return err
	}

	if err := t.pm.WritePage(p); err != nil {
		return err
	}

	return nil
}

func (t *BTree) Search(key int64) (recordID int64, found bool) {
	p, err := t.pm.ReadPage(t.rootID)
	if err != nil {
		return 0, false
	}

	return Search(p, key)
}
