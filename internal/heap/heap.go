// Package heap implements a minimal row heap: a flat, append-only store
// where a RecordID is a physical address (page + slot), not a search
// key. Looking up a row given its RecordID is pure arithmetic plus one
// page read — never a search — mirroring how a Postgres TID resolves
// straight to a heap tuple's physical location.
//
// The heap knows nothing about the B+ tree, and the B+ tree knows
// nothing about the heap: they're connected only by the RecordID
// values the tree stores as leaf entry values and the heap hands back
// from Put.
package heap

import (
	"errors"

	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

var ErrRowTooLarge = errors.New("heap: row too large to fit in a single page")

// Heap is a flat collection of pages holding opaque row bytes, backed
// by its own database file (a separate PageManager from the B+ tree's).
type Heap struct {
	pm         *storage.PageManager
	activePage *page.Page // the page new rows are currently appended to
}

// Open opens (or creates) the heap file at path.
func Open(path string) (*Heap, error) {
	pm, err := storage.Open(path)
	if err != nil {
		return nil, err
	}

	h := &Heap{pm: pm}

	if pm.PageCount() == 0 {
		p := page.NewHeapPage(pm.AllocatePageID())
		if err := pm.WritePage(p); err != nil {
			return nil, err
		}
		h.activePage = p
		return h, nil
	}

	lastPageID := pm.PageCount() - 1
	activePage, err := pm.ReadPage(lastPageID)
	if err != nil {
		return nil, err
	}
	h.activePage = activePage
	return h, nil
}

// Close closes the underlying heap file.
func (h *Heap) Close() error {
	return h.pm.Close()
}

// Put stores rowBytes and returns a RecordID that can later be passed
// to Get to retrieve it. The RecordID encodes a physical address
// (PageID and slot index) — decoding it requires no search.
func (h *Heap) Put(rowBytes []byte) (recordID int64, err error) {
	if page.HeaderSize+page.SlotSize+len(rowBytes) > page.PageSize {
		return 0, ErrRowTooLarge
	}

	if !h.activePage.HasSpaceFor(len(rowBytes)) {
		newPage := page.NewHeapPage(h.pm.AllocatePageID())
		h.activePage = newPage
	}

	slotIndex := h.activePage.NumEntries()
	entryBytes := append([]byte(nil), rowBytes...)
	h.activePage.InsertEntry(slotIndex, entryBytes)
	if err := h.pm.WritePage(h.activePage); err != nil {
		return 0, err
	}

	return int64(uint64(h.activePage.ID)*uint64(page.PageSize) + uint64(slotIndex)), nil
}

// Get retrieves the row bytes previously stored at recordID.
// No search is performed: recordID is decoded directly into a page
// number and slot index.
func (h *Heap) Get(recordID int64) ([]byte, error) {
	pageID := page.PageID(uint64(recordID) / uint64(page.PageSize))
	slotIndex := uint16(uint64(recordID) % uint64(page.PageSize))

	p, err := h.pm.ReadPage(pageID)
	if err != nil {
		return nil, err
	}

	if slotIndex >= p.NumEntries() {
		return nil, errors.New("heap: slot index out of range")
	}

	return append([]byte(nil), p.GetEntry(slotIndex)...), nil
}
