package partial

import (
	"path/filepath"
	"testing"
)

func TestUpsertActiveRowGetsIndexed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 10, RecordID: 100, Status: Active}); err != nil {
		t.Fatalf("Upsert(Active) failed: %v", err)
	}

	got, found := pi.Search(10)
	if !found {
		t.Fatalf("Search(10) not found after inserting an Active row")
	}
	if got != 100 {
		t.Fatalf("Search(10) = %d, want 100", got)
	}
}

func TestUpsertDeletedRowNeverIndexed(t *testing.T) {
	// Record 2 from the spec's own example: key=20, status=DELETED ->
	// not indexed.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 20, RecordID: 200, Status: Deleted}); err != nil {
		t.Fatalf("Upsert(Deleted) failed: %v", err)
	}

	if _, found := pi.Search(20); found {
		t.Fatalf("Search(20) found a row that was never Active")
	}
}

func TestUpsertDeletedToActiveAddsRecord(t *testing.T) {
	// "When a record changes from DELETED to ACTIVE, the index must
	// add the record." — the spec's exact transition.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 30, RecordID: 300, Status: Deleted}); err != nil {
		t.Fatalf("initial Upsert(Deleted) failed: %v", err)
	}
	if _, found := pi.Search(30); found {
		t.Fatalf("Search(30) found before the row ever went Active")
	}

	if err := pi.Upsert(Row{Key: 30, RecordID: 300, Status: Active}); err != nil {
		t.Fatalf("Upsert(Deleted -> Active) failed: %v", err)
	}

	got, found := pi.Search(30)
	if !found {
		t.Fatalf("Search(30) not found after transitioning to Active")
	}
	if got != 300 {
		t.Fatalf("Search(30) = %d, want 300", got)
	}
}

func TestUpsertActiveToDeletedRemovesRecord(t *testing.T) {
	// "When a record changes from ACTIVE to DELETED, the index must
	// remove the record." — the spec's other explicit transition.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 40, RecordID: 400, Status: Active}); err != nil {
		t.Fatalf("initial Upsert(Active) failed: %v", err)
	}
	if _, found := pi.Search(40); !found {
		t.Fatalf("Search(40) not found right after inserting an Active row")
	}

	if err := pi.Upsert(Row{Key: 40, RecordID: 400, Status: Deleted}); err != nil {
		t.Fatalf("Upsert(Active -> Deleted) failed: %v", err)
	}

	if _, found := pi.Search(40); found {
		t.Fatalf("Search(40) still found after transitioning to Deleted")
	}
}

func TestUpsertActiveRowUpdatesRecordID(t *testing.T) {
	// A row can stay Active but have its RecordID change (e.g. the
	// underlying heap row moved). Upsert must reflect the new
	// RecordID, not silently keep serving the stale one.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 50, RecordID: 500, Status: Active}); err != nil {
		t.Fatalf("first Upsert failed: %v", err)
	}
	if err := pi.Upsert(Row{Key: 50, RecordID: 999, Status: Active}); err != nil {
		t.Fatalf("second Upsert (RecordID change) failed: %v", err)
	}

	got, found := pi.Search(50)
	if !found {
		t.Fatalf("Search(50) not found after RecordID update")
	}
	if got != 999 {
		t.Fatalf("Search(50) = %d, want 999 (stale RecordID)", got)
	}
}

func TestUpsertDeletedRowStaysAbsentOnRepeat(t *testing.T) {
	// Deleted -> Deleted (no-op branch) must not error and must not
	// somehow insert the row.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	if err := pi.Upsert(Row{Key: 60, RecordID: 600, Status: Deleted}); err != nil {
		t.Fatalf("first Upsert(Deleted) failed: %v", err)
	}
	if err := pi.Upsert(Row{Key: 60, RecordID: 600, Status: Deleted}); err != nil {
		t.Fatalf("second Upsert(Deleted) failed: %v", err)
	}

	if _, found := pi.Search(60); found {
		t.Fatalf("Search(60) found a row that was Deleted both times")
	}
}

func TestPartialIndexExampleFromSpec(t *testing.T) {
	// Reproduces the spec's own worked example verbatim:
	//   Record 1: key=10, status=ACTIVE   -> indexed
	//   Record 2: key=20, status=DELETED  -> not indexed
	//   Record 3: key=30, status=ACTIVE   -> indexed
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	rows := []Row{
		{Key: 10, RecordID: 1, Status: Active},
		{Key: 20, RecordID: 2, Status: Deleted},
		{Key: 30, RecordID: 3, Status: Active},
	}
	for _, r := range rows {
		if err := pi.Upsert(r); err != nil {
			t.Fatalf("Upsert(%+v) failed: %v", r, err)
		}
	}

	if _, found := pi.Search(10); !found {
		t.Fatalf("Search(10) not found, want indexed")
	}
	if _, found := pi.Search(20); found {
		t.Fatalf("Search(20) found, want not indexed")
	}
	if _, found := pi.Search(30); !found {
		t.Fatalf("Search(30) not found, want indexed")
	}
}

func TestScanOnlyReturnsActiveRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pi, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer pi.Close()

	for i := int64(1); i <= 10; i++ {
		status := Active
		if i%2 == 0 {
			status = Deleted // even keys never make it into the index
		}
		if err := pi.Upsert(Row{Key: i, RecordID: i * 100, Status: status}); err != nil {
			t.Fatalf("Upsert(%d) failed: %v", i, err)
		}
	}

	results, err := pi.Scan(1, 10)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(results) != 5 { // odd keys 1,3,5,7,9
		t.Fatalf("Scan(1,10) returned %d results, want 5 (only Active/odd keys)", len(results))
	}
	for _, r := range results {
		if r.Key%2 == 0 {
			t.Fatalf("Scan returned key %d, which was Deleted and should be absent", r.Key)
		}
		if r.RecordID != r.Key*100 {
			t.Fatalf("Scan result for key %d has RecordID %d, want %d", r.Key, r.RecordID, r.Key*100)
		}
	}
}
