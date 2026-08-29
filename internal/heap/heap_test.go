package heap

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
)

func TestPutAndGetRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	h, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer h.Close()

	row := []byte("hello, row")
	recordID, err := h.Put(row)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := h.Get(recordID)
	if err != nil {
		t.Fatalf("Get(%d) failed: %v", recordID, err)
	}
	if !bytes.Equal(got, row) {
		t.Fatalf("Get(%d) = %q, want %q", recordID, got, row)
	}
}

func TestPutManyRowsAndGetEachBack(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	h, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer h.Close()

	// 40-byte rows: 92 fit in a single page (HeaderSize=21, SlotSize=4,
	// PageSize=4096). 300 rows forces several page boundary crossings,
	// exercising the "activePage is full, allocate a new one" branch
	// repeatedly rather than just once.
	const numRows = 300
	const rowLen = 40

	rowFor := func(i int) []byte {
		b := make([]byte, rowLen)
		copy(b, fmt.Sprintf("row-%d", i))
		return b
	}

	recordIDs := make([]int64, numRows)
	for i := 0; i < numRows; i++ {
		rid, err := h.Put(rowFor(i))
		if err != nil {
			t.Fatalf("Put(row %d) failed: %v", i, err)
		}
		recordIDs[i] = rid
	}

	// Every RecordID must be unique — a collision would mean two rows
	// are silently aliased to the same physical slot.
	seen := make(map[int64]bool, numRows)
	for i, rid := range recordIDs {
		if seen[rid] {
			t.Fatalf("duplicate RecordID %d at row %d", rid, i)
		}
		seen[rid] = true
	}

	// Every row must read back exactly as written, in any order —
	// confirms Get's decode math is correct across every page the
	// writes spilled into, not just the first one.
	for i := numRows - 1; i >= 0; i-- {
		got, err := h.Get(recordIDs[i])
		if err != nil {
			t.Fatalf("Get(row %d, recordID %d) failed: %v", i, recordIDs[i], err)
		}
		if !bytes.Equal(got, rowFor(i)) {
			t.Fatalf("Get(row %d, recordID %d) = %q, want %q", i, recordIDs[i], got, rowFor(i))
		}
	}
}

func TestRecordIDEncodesPageAndSlot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	h, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer h.Close()

	// First row: activePage is the page Open() just allocated (PageID 0
	// per Open's fresh-file branch), first slot.
	rid, err := h.Put([]byte("first"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	wantPageID := page.PageID(0)
	wantSlot := uint16(0)
	wantRecordID := int64(uint64(wantPageID)*uint64(page.PageSize) + uint64(wantSlot))

	if rid != wantRecordID {
		t.Fatalf("first RecordID = %d, want %d (page %d, slot %d)", rid, wantRecordID, wantPageID, wantSlot)
	}

	// Decoding it back by hand (the same arithmetic Get performs)
	// should recover page 0, slot 0.
	gotPageID := page.PageID(uint64(rid) / uint64(page.PageSize))
	gotSlot := uint16(uint64(rid) % uint64(page.PageSize))
	if gotPageID != wantPageID || gotSlot != wantSlot {
		t.Fatalf("decoded RecordID %d as (page %d, slot %d), want (page %d, slot %d)",
			rid, gotPageID, gotSlot, wantPageID, wantSlot)
	}
}

func TestPutRejectsRowLargerThanAPage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	h, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer h.Close()

	// One byte more than what could ever fit in a completely empty page.
	tooBig := make([]byte, page.PageSize-page.HeaderSize-page.SlotSize+1)

	_, err = h.Put(tooBig)
	if err != ErrRowTooLarge {
		t.Fatalf("Put(oversized row) error = %v, want ErrRowTooLarge", err)
	}
}

func TestGetOutOfRangeSlot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	h, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer h.Close()

	if _, err := h.Put([]byte("only row")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Slot 5 was never written on page 0, which holds exactly one entry.
	badRecordID := int64(uint64(0)*uint64(page.PageSize) + 5)
	if _, err := h.Get(badRecordID); err == nil {
		t.Fatalf("Get(%d) succeeded, want an out-of-range error", badRecordID)
	}
}

func TestHeapSurvivesReopenAcrossPageBoundary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.heap")

	rowFor := func(i int) []byte {
		b := make([]byte, 40)
		copy(b, fmt.Sprintf("row-%d", i))
		return b
	}

	h1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	const numRows = 150
	recordIDs := make([]int64, numRows)
	for i := 0; i < numRows; i++ {
		rid, err := h1.Put(rowFor(i))
		if err != nil {
			t.Fatalf("Put(row %d) failed: %v", i, err)
		}
		recordIDs[i] = rid
	}

	if err := h1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	h2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer h2.Close()

	// Every row written before closing must still read back correctly.
	for i := 0; i < numRows; i++ {
		got, err := h2.Get(recordIDs[i])
		if err != nil {
			t.Fatalf("Get(row %d, recordID %d) after reopen failed: %v", i, recordIDs[i], err)
		}
		if !bytes.Equal(got, rowFor(i)) {
			t.Fatalf("Get(row %d) after reopen = %q, want %q", i, got, rowFor(i))
		}
	}

	// A Put after reopening must not collide with any RecordID handed
	// out before closing — this is the real test of whether nextPageID
	// bookkeeping survived the reopen correctly.
	newRID, err := h2.Put(rowFor(numRows))
	if err != nil {
		t.Fatalf("Put after reopen failed: %v", err)
	}
	for i, rid := range recordIDs {
		if rid == newRID {
			t.Fatalf("post-reopen RecordID %d collides with pre-close row %d", newRID, i)
		}
	}
	got, err := h2.Get(newRID)
	if err != nil {
		t.Fatalf("Get after reopen Put failed: %v", err)
	}
	if !bytes.Equal(got, rowFor(numRows)) {
		t.Fatalf("Get after reopen Put = %q, want %q", got, rowFor(numRows))
	}
}
