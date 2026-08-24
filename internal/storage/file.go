// Package storage implements the PageManager: translates PageIDs into
// byte offsets in a single database file, and handles reading/writing
// fixed-size pages to/from disk.
package storage

import (
	"io"
	"os"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// PageManager owns a single open file and knows how to read/write
// fixed-size pages to/from it by PageID.
type PageManager struct {
	file       *os.File
	nextPageID page.PageID
}

// Open opens the database file at path for reading and writing,
// creating it if it doesn't already exist. The existing file size is
// used to infer how many pages it already holds, so subsequent
// AllocatePage calls continue from the correct next PageID rather than
// colliding with pages written in a previous session.
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

// AllocatePage reserves the next unused PageID and returns a fresh leaf
// page with that ID. The page is not written to disk until a caller
// passes it to WritePage — allocation only reserves the ID, it doesn't
// persist anything.
func (pm *PageManager) AllocatePage() *page.Page {
	id := pm.nextPageID
	pm.nextPageID++
	return page.NewLeafPage(id)
}

// ReadPage reads the page with the given PageID from disk, computing
// its byte offset as id * PageSize.
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

// WritePage writes p's bytes to disk at the offset corresponding to
// p.ID, overwriting whatever was previously stored there.
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

// PageCount returns the number of pages that have been allocated so
// far, i.e. the next PageID that AllocatePage would hand out.
func (pm *PageManager) PageCount() page.PageID {
	return pm.nextPageID
}
