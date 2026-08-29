package experiment

import (
	"fmt"
	"path/filepath"

	"github.com/DanielPopoola/index_lab/internal/btree"
	"github.com/DanielPopoola/index_lab/internal/heap"
	"github.com/DanielPopoola/index_lab/internal/partial"
)

// WriteCostResult contains write cost statistics for a scenario.
type WriteCostResult struct {
	Scenario    string
	IndexWrites uint64
	PageSplits  uint64
	PageMerges  uint64
	TotalPages  uint64 // sum of writes across every index and heap
}

// RunWriteCostExperiments measures write cost across four scenarios:
// no index, one index, multiple indexes, and partial index.
func RunWriteCostExperiments(dir string, n int) ([]WriteCostResult, error) {
	var results []WriteCostResult

	noIndexResult, err := runNoIndexScenario(dir, n)
	if err != nil {
		return nil, fmt.Errorf("no-index scenario: %w", err)
	}
	results = append(results, noIndexResult)

	oneIndexResult, err := runOneIndexScenario(dir, n)
	if err != nil {
		return nil, fmt.Errorf("one-index scenario: %w", err)
	}
	results = append(results, oneIndexResult)

	multiIndexResult, err := runMultiIndexScenario(dir, n)
	if err != nil {
		return nil, fmt.Errorf("multi-index scenario: %w", err)
	}
	results = append(results, multiIndexResult)

	partialIndexResult, err := runPartialIndexScenario(dir, n)
	if err != nil {
		return nil, fmt.Errorf("partial-index scenario: %w", err)
	}
	results = append(results, partialIndexResult)

	return results, nil
}

// runNoIndexScenario measures the baseline: rows in heap only.
func runNoIndexScenario(dir string, n int) (WriteCostResult, error) {
	h, err := heap.Open(filepath.Join(dir, "writecost-none-heap.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer h.Close()

	for i := 0; i < n; i++ {
		if _, err := h.Put(fmt.Appendf(nil, "row-%d", i)); err != nil {
			return WriteCostResult{}, err
		}
	}

	return WriteCostResult{
		Scenario:    "no index",
		IndexWrites: 0,
		TotalPages:  h.PageWrites(),
	}, nil
}

// runOneIndexScenario measures cost with one B+ tree index.
func runOneIndexScenario(dir string, n int) (WriteCostResult, error) {
	h, err := heap.Open(filepath.Join(dir, "writecost-one-heap.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer h.Close()

	tree, err := btree.Open(filepath.Join(dir, "writecost-one-index.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer tree.Close()

	for i := 0; i < n; i++ {
		recordID, err := h.Put(fmt.Appendf(nil, "row-%d", i))
		if err != nil {
			return WriteCostResult{}, err
		}
		if err := tree.Insert(int64(i), recordID); err != nil {
			return WriteCostResult{}, err
		}
	}

	stats, err := tree.Stats()
	if err != nil {
		return WriteCostResult{}, err
	}

	return WriteCostResult{
		Scenario:    "one index",
		IndexWrites: stats.PageWrites,
		PageSplits:  stats.PageSplits,
		PageMerges:  stats.PageMerges,
		TotalPages:  h.PageWrites() + stats.PageWrites,
	}, nil
}

// runMultiIndexScenario measures cost with two independent B+ tree indexes.
func runMultiIndexScenario(dir string, n int) (WriteCostResult, error) {
	h, err := heap.Open(filepath.Join(dir, "writecost-multi-heap.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer h.Close()

	tree1, err := btree.Open(filepath.Join(dir, "writecost-multi-index1.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer tree1.Close()

	tree2, err := btree.Open(filepath.Join(dir, "writecost-multi-index2.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer tree2.Close()

	for i := 0; i < n; i++ {
		recordID, err := h.Put(fmt.Appendf(nil, "row-%d", i))
		if err != nil {
			return WriteCostResult{}, err
		}
		if err := tree1.Insert(int64(i), recordID); err != nil {
			return WriteCostResult{}, err
		}
		// Second index over derived key (simulates different column).
		if err := tree2.Insert(int64(n-i), recordID); err != nil {
			return WriteCostResult{}, err
		}
	}

	stats1, err := tree1.Stats()
	if err != nil {
		return WriteCostResult{}, err
	}
	stats2, err := tree2.Stats()
	if err != nil {
		return WriteCostResult{}, err
	}

	return WriteCostResult{
		Scenario:    "multiple indexes (2)",
		IndexWrites: stats1.PageWrites + stats2.PageWrites,
		PageSplits:  stats1.PageSplits + stats2.PageSplits,
		PageMerges:  stats1.PageMerges + stats2.PageMerges,
		TotalPages:  h.PageWrites() + stats1.PageWrites + stats2.PageWrites,
	}, nil
}

// runPartialIndexScenario measures cost with a partial index
// (roughly 50% of rows).
func runPartialIndexScenario(dir string, n int) (WriteCostResult, error) {
	h, err := heap.Open(filepath.Join(dir, "writecost-partial-heap.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer h.Close()

	pi, err := partial.Open(filepath.Join(dir, "writecost-partial-index.db"))
	if err != nil {
		return WriteCostResult{}, err
	}
	defer pi.Close()

	for i := 0; i < n; i++ {
		recordID, err := h.Put(fmt.Appendf(nil, "row-%d", i))
		if err != nil {
			return WriteCostResult{}, err
		}
		status := partial.Deleted
		if i%2 == 0 {
			status = partial.Active // half the rows match the predicate
		}
		if err := pi.Upsert(partial.Row{Key: int64(i), RecordID: recordID, Status: status}); err != nil {
			return WriteCostResult{}, err
		}
	}

	stats, err := pi.Stats()
	if err != nil {
		return WriteCostResult{}, err
	}

	return WriteCostResult{
		Scenario:    "partial index (~50% match predicate)",
		IndexWrites: stats.PageWrites,
		PageSplits:  stats.PageSplits,
		PageMerges:  stats.PageMerges,
		TotalPages:  h.PageWrites() + stats.PageWrites,
	}, nil
}
