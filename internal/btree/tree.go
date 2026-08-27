// tree.go: for now, this only handles operations on a SINGLE leaf page —
// no internal nodes, no splitting yet. This is deliberately the smallest
// possible "real" B+ tree operation: prove Insert/Search work correctly
// before adding the complexity of multiple levels.
package btree

import (
	"encoding/binary"
	"errors"
	"sort"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// Returned by the package-level `Insert` when a single page has no room for a new entry.
var ErrPageFull = errors.New("page full: splitting not yet implemented")

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

// Inserts `(key, recordID)` into `p` directly. Returns `ErrPageFull` if `p` has no room — does not split.
func Insert(p *page.Page, key int64, recordID int64) error {
	encodedKey := EncodeInt64(key)
	entryBytes := append(encodedKey, EncodeInt64(recordID)...)

	index, _ := findKeyIndex(p, encodedKey)

	if !p.HasSpaceFor(len(entryBytes)) {
		return ErrPageFull
	}

	p.InsertEntry(index, entryBytes)
	return nil
}

// Looks up `key` on `p` directly. Does not traverse the tree.
func Search(p *page.Page, key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)
	index, ok := findKeyIndex(p, encodedKey)
	if !ok {
		return 0, false
	}
	valueBytes := p.GetEntry(index)[8:]
	value := DecodeInt64(valueBytes)
	return value, true
}

// Returned by the package-level `Delete` when `key` is not present on `p`.
var ErrKeyNotFound = errors.New("key not found")

// Deletes `key` from `p` directly. Does not traverse the tree and does
// not check or fix underflow — that's the caller's (BTree.Delete's)
// responsibility once this is wired into the tree.
func Delete(p *page.Page, key int64) error {
	encodedKey := EncodeInt64(key)
	index, ok := findKeyIndex(p, encodedKey)
	if !ok {
		return ErrKeyNotFound
	}
	p.DeleteEntry(index)
	return nil
}

// Borrows exactly one entry from `leftSibling` to fix `underflowing`'s
// occupancy. Moves `leftSibling`'s LAST (largest-key) entry into
// `underflowing` — the only entry that can move while keeping both
// pages sorted and keeping every key in `leftSibling` still smaller
// than every key in `underflowing`.
func redistributeFromLeft(underflowing, leftSibling *page.Page) (newSeparator []byte) {
	lastIdx := leftSibling.NumEntries() - 1
	entry := leftSibling.GetEntry(lastIdx)

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
	entry := rightSibling.GetEntry(0)

	rightSibling.DeleteEntry(0)

	underflowing.InsertEntry(underflowing.NumEntries(), entry)

	return rightSibling.GetEntry(0)[:8]
}

// Splits a full leaf page. The smaller half (lower `NumEntries()/2` entries) stays in `oldPage`; the larger half moves to a newly allocated page.
// Sibling pointers (`NextLeafPageID`/`PrevLeafPageID`) are rewired to keep the leaf linked list correct.
// `separatorKey` is the new page's first key — a real, retrievable entry (leaf keys are never discarded).
func splitLeaf(oldPage *page.Page, allocateFn func() *page.Page) (separatorKey []byte, newPage *page.Page, err error) {
	mid := oldPage.NumEntries() / 2

	var moving [][]byte
	for i := mid; i < oldPage.NumEntries(); i++ {
		moving = append(moving, oldPage.GetEntry(i))
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

	separatorKey = oldPage.GetEntry(mid)[:8]
	childBytes := oldPage.GetEntry(mid)[8:]
	middleChildID := page.PageID(binary.BigEndian.Uint64(childBytes))

	var moving [][]byte
	for i := mid + 1; i < oldPage.NumEntries(); i++ {
		moving = append(moving, oldPage.GetEntry(i))
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

// Encodes `id` as 8 big-endian bytes, for use as the value half of an internal-page entry.
func encodeChildID(id page.PageID) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}
