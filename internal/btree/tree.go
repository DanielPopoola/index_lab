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

// ErrPageFull is returned by Insert when a leaf page has no room for a
// new entry. Splitting isn't implemented yet — for now this is an honest
// "not yet supported" signal rather than silently failing or corrupting
// the page.
var ErrPageFull = errors.New("page full: splitting not yet implemented")

// findInsertIndex locates where targetKey belongs in a page's sorted
// slot array via binary search over the key portion of each entry.
// Works for both leaf pages (key -> recordID entries) and internal
// pages (key -> childID entries), since both store the key as the
// first 8 bytes of each entry. found reports whether targetKey already
// exists at that index.
func findInsertIndex(p *page.Page, targetKey []byte) (index uint16, found bool) {
	n := p.NumEntries()

	i := sort.Search(int(n), func(i int) bool {
		return CompareKeys(p.GetEntry(uint16(i))[:8], targetKey) >= 0
	})

	if i < int(n) && CompareKeys(p.GetEntry(uint16(i))[:8], targetKey) == 0 {
		return uint16(i), true
	}

	return uint16(i), false
}

// findChildPageID reports which child of internal page p a search for
// targetKey should descend into: the leftmost child if targetKey is
// smaller than every separator on p, otherwise the child attached to
// the largest separator that is still <= targetKey.
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

// Insert adds a (key, recordID) pair directly to a single page p. It
// does not split p or touch the wider tree structure — it returns
// ErrPageFull if p has no room, leaving splitting to the caller (see
// BTree.Insert, which handles the full tree-level insert-with-split
// flow).
func Insert(p *page.Page, key int64, recordID int64) error {
	encodedKey := EncodeInt64(key)
	entryBytes := append(encodedKey, EncodeInt64(recordID)...)

	index, _ := findInsertIndex(p, encodedKey)

	if !p.HasSpaceFor(len(entryBytes)) {
		return ErrPageFull
	}

	p.InsertEntry(index, entryBytes)
	return nil
}

// Search looks up key on a single leaf page p and reports whether it
// was found. It does not walk the tree — see BTree.Search for the
// root-to-leaf traversal.
func Search(p *page.Page, key int64) (recordID int64, found bool) {
	encodedKey := EncodeInt64(key)
	index, ok := findInsertIndex(p, encodedKey)
	if !ok {
		return 0, false
	}
	valueBytes := p.GetEntry(index)[8:]
	value := DecodeInt64(valueBytes)
	return value, true
}

// splitLeaf splits a full leaf page in two: the smaller half stays in
// oldPage, the larger half moves to a newly allocated page. Sibling
// pointers (NextLeafPageID/PrevLeafPageID) are rewired so range scans
// stay correct. Unlike splitInternal, the separator key here is a real
// entry that stays retrievable in newPage — leaf keys can't be
// discarded, since leaves are where all actual data lives.
//
// allocateFn is injected (rather than calling a PageManager directly)
// so this function can be tested without a real file/PageManager, using
// a fake closure-based allocator.
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

// splitInternal splits a full internal page in three parts: entries
// before the midpoint stay in oldPage, entries after it move to a
// newly allocated page, and the midpoint entry itself is consumed
// entirely — its key is promoted OUT as separatorKey rather than kept
// in either half (unlike splitLeaf), since internal pages exist only to
// route searches and don't need every key to remain retrievable. The
// midpoint entry's child pointer becomes newPage's leftmost child, the
// same "unattached, visited when nothing else matches" role every
// internal page's leftmost child plays.
//
// allocateFn is injected for the same testability reason as in
// splitLeaf; only reserved.ID is used from its result, since
// page.NewInternalPage builds a fresh, distinct page rather than
// converting the one allocateFn returns.
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

// encodeChildID turns id into its 8-byte big-endian form, for storing
// as the second half of an internal-page entry (key(8) + childID(8)).
func encodeChildID(id page.PageID) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}
