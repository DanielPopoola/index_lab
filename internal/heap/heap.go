// Package heap implements an append-only row store.
package heap

import (
	"errors"

	"github.com/DanielPopoola/index_lab/internal/buffer"
	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

var ErrRowTooLarge = errors.New("heap: row too large to fit in a single page")

// Heap is a collection of pages that store opaque row bytes.
type Heap struct {
	pm         *storage.PageManager
	pool       *buffer.Pool // nil unless WithBufferPool was passed to Open
	activePage *page.Page   // the page new rows are currently appended to
}

// Option configures a Heap at Open time.
type Option func(*Heap)

// WithBufferPool enables an LRU page cache for the heap.
func WithBufferPool(capacity int) Option {
	return func(h *Heap) {
		h.pool = buffer.NewPool(h.pm, capacity)
	}
}

// Open creates or opens the heap file at path.
func Open(path string, opts ...Option) (*Heap, error) {
	pm, err := storage.Open(path)
	if err != nil {
		return nil, err
	}

	h := &Heap{pm: pm}
	for _, opt := range opts {
		opt(h)
	}

	if pm.PageCount() == 0 {
		p := page.NewHeapPage(pm.AllocatePageID())
		if err := h.writePage(p); err != nil {
			return nil, err
		}
		h.activePage = p
		return h, nil
	}

	lastPageID := pm.PageCount() - 1
	activePage, err := h.readPage(lastPageID)
	if err != nil {
		return nil, err
	}
	h.activePage = activePage
	return h, nil
}

// readPage loads a page by ID.
//
// If a buffer pool is configured, it reads through that cache; otherwise it
// reads directly from the PageManager.
func (h *Heap) readPage(id page.PageID) (*page.Page, error) {
	if h.pool != nil {
		return h.pool.GetPage(id)
	}
	return h.pm.ReadPage(id)
}

// writePage persists p to storage.
//
// If a buffer pool is configured, the page is cached and marked dirty; the
// actual disk write is deferred until eviction or Close. Without a pool, the
// page is written directly to the PageManager.
func (h *Heap) writePage(p *page.Page) error {
	if h.pool != nil {
		return h.pool.Put(p.ID, p)
	}
	return h.pm.WritePage(p)
}

// Close closes the heap.
//
// If a buffer pool is configured, it flushes dirty pages before closing the
// underlying PageManager.
func (h *Heap) Close() error {
	if h.pool != nil {
		return h.pool.Close()
	}
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

// ResetStats clears the read and write counters.
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
	if err := h.writePage(h.activePage); err != nil {
		return 0, err
	}

	return int64(uint64(h.activePage.ID)*uint64(page.PageSize) + uint64(slotIndex)), nil
}

// Get retrieves the row bytes previously stored at recordID.
func (h *Heap) Get(recordID int64) ([]byte, error) {
	pageID := page.PageID(uint64(recordID) / uint64(page.PageSize))
	slotIndex := uint16(uint64(recordID) % uint64(page.PageSize))

	p, err := h.readPage(pageID)
	if err != nil {
		return nil, err
	}

	if slotIndex >= p.NumEntries() {
		return nil, errors.New("heap: slot index out of range")
	}

	return append([]byte(nil), p.GetEntry(slotIndex)...), nil
}
