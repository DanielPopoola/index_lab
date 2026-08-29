// tree.go: forthis only handles operations on a SINGLE leaf page —
// no internal nodes, no splitting yet. This is deliberately the smallest
// possible "real" B+ tree operation: prove Insert/Search work correctly
// before adding the complexity of multiple levels.
package btree

import (
	"errors"

	"github.com/DanielPopoola/index_lab/internal/page"
)

var ErrPageFull = errors.New("page full: splitting not yet implemented")
var ErrKeyNotFound = errors.New("key not found")

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

// Deletes `key` from `p` directly
func Delete(p *page.Page, key int64) error {
	encodedKey := EncodeInt64(key)
	index, ok := findKeyIndex(p, encodedKey)
	if !ok {
		return ErrKeyNotFound
	}
	p.DeleteEntry(index)
	return nil
}
