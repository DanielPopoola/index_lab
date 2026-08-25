// Package storage implements the PageManager: translates PageIDs into
// byte offsets in a single database file, and handles reading/writing
// fixed-size pages to/from disk.
package storage

import (
	"io"
	"os"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// Owns a single open file and translates `PageID`s into byte offsets for reading/writing fixed-size pages.
type PageManager struct {
	file       *os.File
	nextPageID page.PageID
}

// Opens the database file at `path`, creating it if absent.
// Infers the next allocatable `PageID` from the existing file size (`fileInfo.Size() / PageSize`).
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

// Reserves the next unused `PageID` and returns a fresh leaf page with that ID.
func (pm *PageManager) AllocatePage() *page.Page {
	id := pm.nextPageID
	pm.nextPageID++
	return page.NewLeafPage(id)
}

// Reads the page at `id` from disk
func (pm *PageManager) ReadPage(id page.PageID) (*page.Page, error) {
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

// Writes `p.Data` to disk at the offset corresponding to `p.ID`, overwriting any existing contents there.
func (pm *PageManager) WritePage(p *page.Page) error {
	offset := int64(p.ID) * page.PageSize
	if _, err := pm.file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if _, err := pm.file.Write(p.Data[:]); err != nil {
		return err
	}
	return nil
}

// Returns the number of pages allocated so far (equivalently, the next `PageID` `AllocatePage` would hand out).
func (pm *PageManager) PageCount() page.PageID {
	return pm.nextPageID
}

// Overrides the next `PageID` that `AllocatePage` will hand out.
func (pm *PageManager) SetNextPageID(id page.PageID) {
	pm.nextPageID = id
}
