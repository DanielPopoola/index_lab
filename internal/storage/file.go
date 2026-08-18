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

func (pm *PageManager) Close() error {
	return pm.file.Close()
}

func (pm *PageManager) AllocatePage() *page.Page {
	id := pm.nextPageID
	pm.nextPageID++
	return page.NewLeafPage(id)
}

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

func (pm *PageManager) PageCount() page.PageID {
	return pm.nextPageID
}
