package experiment

import (
	"fmt"
	"path/filepath"

	"github.com/DanielPopoola/index_lab/internal/btree"
	"github.com/DanielPopoola/index_lab/internal/heap"
)

// ReadResult contains page read statistics for an operation, broken down
// by index and heap pages.
type ReadResult struct {
	Operation      string
	IndexPageReads uint64
	HeapPageReads  uint64
	TotalPages     uint64
}

// RunReadExperiments measures pages touched by three operations:
// point lookup, range scan, and secondary-index lookup (which follows
// a RecordID from the index into the heap). Uses sequential keys.
func RunReadExperiments(dir string, n int) ([]ReadResult, error) {
	treePath := filepath.Join(dir, "reads-index.db")
	heapPath := filepath.Join(dir, "reads-heap.db")

	tree, err := btree.Open(treePath)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	h, err := heap.Open(heapPath)
	if err != nil {
		return nil, err
	}
	defer h.Close()

	// Populate: tree stores key -> RecordID, heap stores RecordID -> row.
	var midKeyRecordID int64
	mid := n / 2
	for i := 0; i < n; i++ {
		key := int64(i)
		row := []byte(fmt.Sprintf("row-%d", i))
		recordID, err := h.Put(row)
		if err != nil {
			return nil, err
		}
		if err := tree.Insert(key, recordID); err != nil {
			return nil, err
		}
		if i == mid {
			midKeyRecordID = recordID
		}
	}

	tree.ResetStats()

	var results []ReadResult

	// 1. Point lookup: Search for one key in the middle of the range.
	if _, found := tree.Search(int64(mid)); !found {
		return nil, fmt.Errorf("point lookup setup problem: key %d not found", mid)
	}
	pointStats, err := tree.Stats()
	if err != nil {
		return nil, err
	}
	results = append(results, ReadResult{
		Operation:      "point lookup",
		IndexPageReads: pointStats.PageReads,
		TotalPages:     pointStats.PageReads,
	})

	tree.ResetStats()

	// 2. Range scan: a contiguous slice of 100 keys around the middle.
	scanStart := int64(mid - 50)
	scanEnd := int64(mid + 50)
	if scanStart < 0 {
		scanStart = 0
	}
	scanResults, err := tree.Scan(scanStart, scanEnd)
	if err != nil {
		return nil, err
	}
	if len(scanResults) == 0 {
		return nil, fmt.Errorf("range scan setup problem: got 0 results for [%d, %d]", scanStart, scanEnd)
	}
	scanStats, err := tree.Stats()
	if err != nil {
		return nil, err
	}
	results = append(results, ReadResult{
		Operation:      "range scan (100 keys)",
		IndexPageReads: scanStats.PageReads,
		TotalPages:     scanStats.PageReads,
	})

	tree.ResetStats()

	// 3. Secondary-index lookup: search tree then follow to heap.
	gotRecordID, found := tree.Search(int64(mid))
	if !found || gotRecordID != midKeyRecordID {
		return nil, fmt.Errorf("secondary-index lookup setup problem: Search(%d) = (%d, %v), want (%d, true)", mid, gotRecordID, found, midKeyRecordID)
	}
	indexStats, err := tree.Stats()
	if err != nil {
		return nil, err
	}

	if _, err := h.Get(gotRecordID); err != nil {
		return nil, fmt.Errorf("heap.Get(%d) failed: %w", gotRecordID, err)
	}
	heapPageReads := h.PageReads()

	results = append(results, ReadResult{
		Operation:      "secondary-index lookup (index + heap)",
		IndexPageReads: indexStats.PageReads,
		HeapPageReads:  heapPageReads,
		TotalPages:     indexStats.PageReads + heapPageReads,
	})

	return results, nil
}
