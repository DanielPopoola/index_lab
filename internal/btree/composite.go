package btree

import (
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

// CompositeBTree is a B+ tree configured for 16-byte composite keys.
type CompositeBTree struct {
	tree *BTree
}

// OpenComposite opens or creates a composite-key B+ tree at path.
func OpenComposite(path string) (*CompositeBTree, error) {
	pm, err := storage.Open(path)
	if err != nil {
		return nil, err
	}

	var rootID page.PageID
	if pm.PageCount() == 0 {
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

	return &CompositeBTree{tree: &BTree{pm: pm, rootID: rootID, entrySize: CompositeEntrySize}}, nil
}

// Close closes the underlying PageManager.
func (c *CompositeBTree) Close() error {
	return c.tree.Close()
}

// Insert inserts (columnA, columnB) -> recordID.
func (c *CompositeBTree) Insert(columnA, columnB, recordID int64) error {
	encodedKey := EncodeCompositeKey(columnA, columnB)
	leaf, ancestors, err := c.tree.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	if err := InsertComposite(leaf, columnA, columnB, recordID); err != nil {
		if err == ErrPageFull {
			separatorKey, newLeaf, splitErr := splitLeaf(leaf, c.tree.pm.AllocatePage)
			if splitErr != nil {
				return splitErr
			}
			if err := c.tree.updateNextLeafPrevLink(newLeaf); err != nil {
				return err
			}

			if CompareKeys(encodedKey, separatorKey) >= 0 {
				if err := InsertComposite(newLeaf, columnA, columnB, recordID); err != nil {
					return err
				}
			} else {
				if err := InsertComposite(leaf, columnA, columnB, recordID); err != nil {
					return err
				}
			}
			if err := c.tree.pm.WritePage(leaf); err != nil {
				return err
			}
			if err := c.tree.pm.WritePage(newLeaf); err != nil {
				return err
			}
			return c.tree.propagateSplit(leaf, newLeaf, separatorKey, ancestors)
		}
		return err
	}
	return c.tree.pm.WritePage(leaf)
}

// Search looks up (columnA, columnB).
func (c *CompositeBTree) Search(columnA, columnB int64) (recordID int64, found bool) {
	encodedKey := EncodeCompositeKey(columnA, columnB)
	leaf, _, err := c.tree.findLeafWithPath(encodedKey)
	if err != nil {
		return 0, false
	}

	index, ok := findKeyIndex(leaf, encodedKey)
	if !ok {
		return 0, false
	}
	entry := leaf.GetEntry(index)
	if CompareKeys(entry[:16], encodedKey) != 0 {
		return 0, false
	}
	return DecodeInt64(entry[16:]), true
}

// Delete removes (columnA, columnB).
func (c *CompositeBTree) Delete(columnA, columnB int64) error {
	encodedKey := EncodeCompositeKey(columnA, columnB)
	leaf, ancestors, err := c.tree.findLeafWithPath(encodedKey)
	if err != nil {
		return err
	}

	index, ok := findKeyIndex(leaf, encodedKey)
	if !ok || CompareKeys(leaf.GetEntry(index)[:16], encodedKey) != 0 {
		return ErrKeyNotFound
	}
	leaf.DeleteEntry(index)

	if len(ancestors) == 0 {
		return c.tree.pm.WritePage(leaf)
	}
	if leaf.NumEntries() >= page.MinEntries(c.tree.entrySize) {
		return c.tree.pm.WritePage(leaf)
	}
	return c.tree.fixLeafUnderflow(leaf, ancestors)
}

// CompositeScanResult is one ((columnA, columnB), recordID) result
// from Scan.
type CompositeScanResult struct {
	ColumnA  int64
	ColumnB  int64
	RecordID int64
}

// Scan returns every entry with columnA == columnA and
// startColumnB <= columnB <= endColumnB, in ascending columnB order.
//
// Unlike BTree.Scan (a full range over one flat key space), this scan
// is deliberately narrower: fixed columnA, ranging columnB, mirroring
// exactly what the task spec asks Scan to prove — that (columnA,
// columnB) can efficiently locate a contiguous range constrained by
// columnA, because every entry sharing that columnA sits together in
// key order, adjacent to each other.
func (c *CompositeBTree) Scan(columnA, startColumnB, endColumnB int64) ([]CompositeScanResult, error) {
	if startColumnB > endColumnB {
		return nil, ErrInvalidRange
	}

	startKey := EncodeCompositeKey(columnA, startColumnB)
	endKey := EncodeCompositeKey(columnA, endColumnB)

	leaf, _, err := c.tree.findLeafWithPath(startKey)
	if err != nil {
		return nil, err
	}

	var results []CompositeScanResult
	idx, _ := findKeyIndex(leaf, startKey)

	for {
		for idx < leaf.NumEntries() {
			entry := leaf.GetEntry(idx)
			key := entry[:16]
			if CompareKeys(key, endKey) > 0 {
				return results, nil
			}

			colA, colB := DecodeCompositeKey(key)
			results = append(results, CompositeScanResult{
				ColumnA:  colA,
				ColumnB:  colB,
				RecordID: DecodeInt64(entry[16:]),
			})
			idx++
		}
		nextID := leaf.NextLeafPageID()
		if nextID == 0 {
			return results, nil
		}
		leaf, err = c.tree.pm.ReadPage(nextID)
		if err != nil {
			return nil, err
		}
		idx = 0
	}
}

func InsertComposite(p *page.Page, columnA, columnB, recordID int64) error {
	encodedKey := EncodeCompositeKey(columnA, columnB)
	encodedRecordID := EncodeInt64(recordID)

	entryBytes := append(append([]byte(nil), encodedKey...), encodedRecordID...)

	index, _ := findKeyIndex(p, encodedKey)
	if !p.HasSpaceFor(len(entryBytes)) {
		return ErrPageFull
	}
	p.InsertEntry(index, entryBytes)
	return nil
}
