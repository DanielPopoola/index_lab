package experiment

import (
	"testing"
)

func TestSequentialKeysAreStrictlyIncreasing(t *testing.T) {
	keys := GenerateKeys(Sequential, 1000)
	if len(keys) != 1000 {
		t.Fatalf("got %d keys, want 1000", len(keys))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] <= keys[i-1] {
			t.Fatalf("Sequential keys not strictly increasing at index %d: %d <= %d", i, keys[i], keys[i-1])
		}
	}
}

func TestUUID7LikeKeysAreMostlyIncreasing(t *testing.T) {
	// UUID7-like keys carry a millisecond timestamp in the high bits,
	// so consecutive keys should be non-decreasing almost all the
	// time — occasional equal-millisecond ties are fine (that's what
	// the random low bits are for), but a LATER key sorting BEFORE an
	// earlier one should essentially never happen within a fast loop.
	keys := GenerateKeys(UUID7Like, 1000)
	if len(keys) != 1000 {
		t.Fatalf("got %d keys, want 1000", len(keys))
	}

	inversions := 0
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			inversions++
		}
	}
	// Allow a small number of inversions from clock granularity/jitter,
	// but this must look nothing like Random's behavior (which would
	// have inversions roughly half the time).
	if inversions > len(keys)/10 {
		t.Fatalf("UUID7Like had %d inversions out of %d keys, want a small fraction (locality should hold)", inversions, len(keys))
	}
}

func TestRandomKeysAreNotSorted(t *testing.T) {
	// The inverse check: Random keys should NOT come out mostly
	// increasing — if they did, either the generator is broken or it's
	// accidentally reusing UUID7Like's logic.
	keys := GenerateKeys(Random, 1000)
	if len(keys) != 1000 {
		t.Fatalf("got %d keys, want 1000", len(keys))
	}

	inversions := 0
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			inversions++
		}
	}
	// Truly random ordering should have inversions roughly half the
	// time; a generous lower bound rules out "accidentally sorted."
	if inversions < len(keys)/4 {
		t.Fatalf("Random had only %d inversions out of %d keys, want roughly half (looks too ordered)", inversions, len(keys))
	}
}

func TestGeneratedKeysHaveNoDuplicates(t *testing.T) {
	// Not a hard guarantee for any of these generators, but with 5000
	// keys drawn from a 64-bit (or effectively wide) space, a
	// collision would be so improbable that seeing one suggests a
	// real bug (e.g. UUID7Like's millisecond+counter math wrapping).
	for _, pattern := range []KeyPattern{Sequential, Random, UUID4Like, UUID7Like} {
		keys := GenerateKeys(pattern, 5000)
		seen := make(map[int64]bool, len(keys))
		for _, k := range keys {
			if seen[k] {
				t.Fatalf("pattern %s produced a duplicate key: %d", pattern, k)
			}
			seen[k] = true
		}
	}
}

func TestRunWorkloadReportsConsistentStats(t *testing.T) {
	dir := t.TempDir()

	result, err := RunWorkload(dir, Sequential, 2000)
	if err != nil {
		t.Fatalf("RunWorkload failed: %v", err)
	}

	if result.InsertCount != 2000 {
		t.Fatalf("InsertCount = %d, want 2000", result.InsertCount)
	}
	if result.PageSplits == 0 {
		t.Fatalf("PageSplits = 0, want > 0 after 2000 sequential inserts")
	}
	if result.FinalHeight < 2 {
		t.Fatalf("FinalHeight = %d, want >= 2 after 2000 inserts", result.FinalHeight)
	}
	if result.FinalPageCount <= 1 {
		t.Fatalf("FinalPageCount = %d, want > 1", result.FinalPageCount)
	}
	if result.PageWrites == 0 {
		t.Fatalf("PageWrites = 0, want > 0")
	}
}

func TestRunAllWorkloadsCoversEveryPattern(t *testing.T) {
	dir := t.TempDir()

	results, err := RunAllWorkloads(dir, 500)
	if err != nil {
		t.Fatalf("RunAllWorkloads failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4 (one per KeyPattern)", len(results))
	}

	seen := make(map[KeyPattern]bool)
	for _, r := range results {
		if r.InsertCount != 500 {
			t.Fatalf("pattern %s: InsertCount = %d, want 500", r.Pattern, r.InsertCount)
		}
		seen[r.Pattern] = true
	}
	for _, p := range []KeyPattern{Sequential, Random, UUID4Like, UUID7Like} {
		if !seen[p] {
			t.Fatalf("pattern %s missing from RunAllWorkloads results", p)
		}
	}
}

func TestRunReadExperimentsProducesThreeOperations(t *testing.T) {
	dir := t.TempDir()

	results, err := RunReadExperiments(dir, 2000)
	if err != nil {
		t.Fatalf("RunReadExperiments failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3 (point lookup, range scan, secondary-index lookup)", len(results))
	}

	var point, scan, secondary *ReadResult
	for i := range results {
		switch results[i].Operation {
		case "point lookup":
			point = &results[i]
		case "range scan (100 keys)":
			scan = &results[i]
		case "secondary-index lookup (index + heap)":
			secondary = &results[i]
		}
	}
	if point == nil || scan == nil || secondary == nil {
		t.Fatalf("missing expected operation in results: %+v", results)
	}

	// A 100-key range scan should touch at least as many index pages
	// as a single point lookup — scanning necessarily does at least as
	// much work as finding one key.
	if scan.IndexPageReads < point.IndexPageReads {
		t.Fatalf("range scan touched fewer index pages (%d) than point lookup (%d)", scan.IndexPageReads, point.IndexPageReads)
	}

	// The secondary-index lookup must show a nonzero heap read — that
	// extra hop is the entire point of section 13's comparison.
	if secondary.HeapPageReads == 0 {
		t.Fatalf("secondary-index lookup HeapPageReads = 0, want > 0")
	}
	if secondary.TotalPages != secondary.IndexPageReads+secondary.HeapPageReads {
		t.Fatalf("secondary lookup TotalPages = %d, want IndexPageReads(%d) + HeapPageReads(%d)",
			secondary.TotalPages, secondary.IndexPageReads, secondary.HeapPageReads)
	}
}

func TestRunWriteCostExperimentsShowsIncreasingCostWithMoreIndexes(t *testing.T) {
	dir := t.TempDir()

	results, err := RunWriteCostExperiments(dir, 1000)
	if err != nil {
		t.Fatalf("RunWriteCostExperiments failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4 (no index, one index, multiple indexes, partial index)", len(results))
	}

	byScenario := make(map[string]WriteCostResult, len(results))
	for _, r := range results {
		byScenario[r.Scenario] = r
	}

	noIndex, ok := byScenario["no index"]
	if !ok {
		t.Fatalf("missing 'no index' scenario")
	}
	oneIndex, ok := byScenario["one index"]
	if !ok {
		t.Fatalf("missing 'one index' scenario")
	}
	multiIndex, ok := byScenario["multiple indexes (2)"]
	if !ok {
		t.Fatalf("missing 'multiple indexes (2)' scenario")
	}
	partialIndex, ok := byScenario["partial index (~50% match predicate)"]
	if !ok {
		t.Fatalf("missing partial index scenario")
	}

	// The core claim section 14 wants demonstrated: total pages
	// affected should increase as more indexes are maintained.
	if oneIndex.TotalPages <= noIndex.TotalPages {
		t.Fatalf("one index TotalPages (%d) not greater than no-index baseline (%d)", oneIndex.TotalPages, noIndex.TotalPages)
	}
	if multiIndex.TotalPages <= oneIndex.TotalPages {
		t.Fatalf("multi-index TotalPages (%d) not greater than one-index (%d)", multiIndex.TotalPages, oneIndex.TotalPages)
	}

	// A partial index over ~50% of rows should write meaningfully less
	// than a full index over all rows.
	if partialIndex.IndexWrites >= oneIndex.IndexWrites {
		t.Fatalf("partial index writes (%d) not less than full one-index writes (%d)", partialIndex.IndexWrites, oneIndex.IndexWrites)
	}
}
