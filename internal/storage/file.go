// Package storage implements the PageManager: translates PageIDs into
// byte offsets in a single database file, and handles reading/writing
// fixed-size pages to/from disk.
package storage

import (
	"io"
	"os"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// PageManager owns a single open file and translates PageIDs into byte offsets.
type PageManager struct {
	file       *os.File
	nextPageID page.PageID
	pageReads  uint64
	pageWrites uint64
}

// Open opens the database file at path, creating it if absent.
func Open(path string) (*PageManager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	pageCount := fileInfo.Size() / page.PageSize
	pm := &PageManager{
		file:       file,
		nextPageID: page.PageID(pageCount),
	}
	return pm, nil
}

// Close closes the underlying database file.
func (pm *PageManager) Close() error {
	return pm.file.Close()
}

// AllocatePageID reserves and returns the next unused PageID.
func (pm *PageManager) AllocatePageID() page.PageID {
	id := pm.nextPageID
	pm.nextPageID++
	return id
}

// AllocatePage reserves the next unused PageID and returns a fresh leaf page.
func (pm *PageManager) AllocatePage() *page.Page {
	return page.NewLeafPage(pm.AllocatePageID())
}

// ReadPage reads the page at id from disk.
func (pm *PageManager) ReadPage(id page.PageID) (*page.Page, error) {
	pm.pageReads++

	offset := int64(id) * page.PageSize
	if _, err := pm.file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	p := &page.Page{ID: id}
	if _, err := io.ReadFull(pm.file, p.Data[:]); err != nil {
		return nil, err
	}
	return p, nil
}

// WritePage writes p to disk at the offset corresponding to p.ID.
func (pm *PageManager) WritePage(p *page.Page) error {
	pm.pageWrites++

	offset := int64(p.ID) * page.PageSize
	if _, err := pm.file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if _, err := pm.file.Write(p.Data[:]); err != nil {
		return err
	}
	return nil
}

// PageReads returns the number of ReadPage calls made so far.
func (pm *PageManager) PageReads() uint64 {
	return pm.pageReads
}

// PageWrites returns the number of WritePage calls made so far.
func (pm *PageManager) PageWrites() uint64 {
	return pm.pageWrites
}

// ResetStats zeroes the read/write counters, without affecting
// nextPageID or any on-disk content — for starting a fresh
// measurement window between experiments.
func (pm *PageManager) ResetStats() {
	pm.pageReads = 0
	pm.pageWrites = 0
}

// PageCount returns the number of pages allocated so far.
func (pm *PageManager) PageCount() page.PageID {
	return pm.nextPageID
}

// SetNextPageID sets the next PageID that AllocatePage will hand out.
func (pm *PageManager) SetNextPageID(id page.PageID) {
	pm.nextPageID = id
}
