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

// findInsertIndex locates where a key belongs in a leaf page's sorted
// slot array via binary search over the KEY portion of each entr
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

// encodeChildID turns a page.PageID into its 8-byte big-endian form, for
// storing as the second half of an internal-page entry.
func encodeChildID(id page.PageID) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}
