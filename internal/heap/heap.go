// Package heap implements a flat, append-only store where RecordID is a physical address.
package heap

import (
	"errors"

	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

var ErrRowTooLarge = errors.New("heap: row too large to fit in a single page")

// Heap is a collection of pages holding opaque row bytes.
type Heap struct {
	pm         *storage.PageManager
	activePage *page.Page // the page new rows are currently appended to
}

// Open opens or creates the heap file at path.
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

// Close closes the underlying file.
func (h *Heap) Close() error {
	return h.pm.Close()
}

// PageReads returns the number of page reads performed so far.
func (h *Heap) PageReads() uint64 {
	return h.pm.PageReads()
}

// PageWrites returns the number of page writes performed so far.
func (h *Heap) PageWrites() uint64 {
	return h.pm.PageWrites()
}

// ResetStats zeroes the read/write counters, for starting a fresh
// measurement window between experiments.
func (h *Heap) ResetStats() {
	h.pm.ResetStats()
}

// Put stores rowBytes and returns a RecordID for later retrieval.
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
