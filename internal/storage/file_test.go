package storage

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
)

func TestPageSurvivesReopen(t *testing.T) {
	// t.TempDir() gives a fresh directory, auto-deleted when the test ends.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	entryBytes := []byte{1, 2, 3, 4}

	pm, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	p := pm.AllocatePage()
	id := p.ID

	p.InsertEntry(0, entryBytes)

	if err := pm.WritePage(p); err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	if err := pm.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	pm2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pm2.Close()

	got, err := pm2.ReadPage(id)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	if got.NumEntries() != 1 {
		t.Errorf("NumEntries = %d, want 1", got.NumEntries())
	}

	gotEntry := got.GetEntry(0)
	if !bytes.Equal(gotEntry, entryBytes) {
		t.Errorf("GetEntry(0) = %v, want %v", gotEntry, entryBytes)
	}

	if got.PageType() != page.LeafPage {
		t.Errorf("PageType = %v, want LeafPage", got.PageType())
	}
}
