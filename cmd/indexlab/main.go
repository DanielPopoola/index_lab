package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/DanielPopoola/index_lab/internal/experiment"
)

func main() {
	var (
		outDir       = flag.String("out", "", "directory for experiment database files (default: a temp dir, removed after the run)")
		n            = flag.Int("n", 5000, "number of keys/rows per experiment")
		poolCapacity = flag.Int("pool-capacity", 64, "buffer pool capacity (pages) for the buffer-pool comparison")
		syncInterval = flag.Int("sync-interval", 100, "sync every N inserts, for the sync-cost comparison")
		which        = flag.String("run", "all", "which experiment(s) to run: all, workload, reads, writes, bufferpool, sync")
	)
	flag.Parse()

	dir := *outDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "indexlab-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	} else if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}

	runners := map[string]func(dir string, n, poolCapacity, syncInterval int) error{
		"workload":   runWorkloadExperiment,
		"reads":      runReadsExperiment,
		"writes":     runWritesExperiment,
		"bufferpool": runBufferPoolExperiment,
		"sync":       runSyncExperiment,
	}

	order := []string{"workload", "reads", "writes", "bufferpool", "sync"}

	var toRun []string
	if *which == "all" {
		toRun = order
	} else if _, ok := runners[*which]; ok {
		toRun = []string{*which}
	} else {
		fmt.Fprintf(os.Stderr, "unknown -run value %q (want: all, workload, reads, writes, bufferpool, sync)\n", *which)
		os.Exit(1)
	}

	for _, name := range toRun {
		if err := runners[name](dir, *n, *poolCapacity, *syncInterval); err != nil {
			fmt.Fprintf(os.Stderr, "%s experiment failed: %v\n", name, err)
			os.Exit(1)
		}
	}
}

func runWorkloadExperiment(dir string, n, _, _ int) error {
	results, err := experiment.RunAllWorkloads(dir, n)
	if err != nil {
		return err
	}

	fmt.Println("=== Workload experiment (key-generation pattern vs. tree shape) ===")
	w := newTable("PATTERN", "INSERTS", "SPLITS", "WRITES", "HEIGHT", "PAGES")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\n",
			r.Pattern, r.InsertCount, r.PageSplits, r.PageWrites, r.FinalHeight, r.FinalPageCount)
	}
	return flushTable(w)
}

func runReadsExperiment(dir string, n, _, _ int) error {
	results, err := experiment.RunReadExperiments(dir, n)
	if err != nil {
		return err
	}

	fmt.Println("=== Read experiment (pages touched per operation) ===")
	w := newTable("OPERATION", "INDEX READS", "HEAP READS", "TOTAL")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", r.Operation, r.IndexPageReads, r.HeapPageReads, r.TotalPages)
	}
	return flushTable(w)
}

func runWritesExperiment(dir string, n, _, _ int) error {
	results, err := experiment.RunWriteCostExperiments(dir, n)
	if err != nil {
		return err
	}

	fmt.Println("=== Write-cost experiment (indexing overhead) ===")
	w := newTable("SCENARIO", "INDEX WRITES", "SPLITS", "MERGES", "TOTAL PAGES")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", r.Scenario, r.IndexWrites, r.PageSplits, r.PageMerges, r.TotalPages)
	}
	return flushTable(w)
}

func runBufferPoolExperiment(dir string, n, poolCapacity, _ int) error {
	results, err := experiment.RunBufferPoolComparison(dir, experiment.Random, n, poolCapacity)
	if err != nil {
		return err
	}

	fmt.Println("=== Buffer pool experiment (deferred writes vs. write-through) ===")
	w := newTable("LABEL", "INSERTS", "PAGE READS", "PAGE WRITES", "DURATION")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", r.Label, r.InsertCount, r.PageReads, r.PageWrites, r.Duration)
	}
	return flushTable(w)
}

func runSyncExperiment(dir string, n, _, syncInterval int) error {
	results, err := experiment.RunSyncCostExperiments(dir, n, syncInterval)
	if err != nil {
		return err
	}

	fmt.Println("=== Sync-cost experiment (durability boundary overhead) ===")
	w := newTable("STRATEGY", "INSERTS", "SYNC CALLS", "DURATION")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", r.Strategy, r.InsertCount, r.SyncCalls, r.Duration)
	}
	return flushTable(w)
}

// newTable starts a tab-aligned table with the given header columns.
func newTable(headers ...string) *tabwriter.Writer {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	return w
}

// flushTable writes the table out and adds a trailing blank line to
// separate it from the next experiment's output.
func flushTable(w *tabwriter.Writer) error {
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println()
	return nil
}
