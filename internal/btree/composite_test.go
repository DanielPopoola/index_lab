package btree

import (
	"path/filepath"
	"testing"
)

func TestCompositeInsertAndSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	type entry struct{ colA, colB, recordID int64 }
	entries := []entry{
		{1, 10, 100},
		{1, 20, 200},
		{1, 30, 300},
		{2, 10, 400},
		{2, 20, 500},
		{3, 10, 600},
	}

	for _, e := range entries {
		if err := tree.Insert(e.colA, e.colB, e.recordID); err != nil {
			t.Fatalf("Insert(%d, %d) failed: %v", e.colA, e.colB, err)
		}
	}

	for _, e := range entries {
		rid, found := tree.Search(e.colA, e.colB)
		if !found {
			t.Fatalf("Search(%d, %d) not found", e.colA, e.colB)
		}
		if rid != e.recordID {
			t.Fatalf("Search(%d, %d) = %d, want %d", e.colA, e.colB, rid, e.recordID)
		}
	}

	if _, found := tree.Search(1, 99); found {
		t.Fatalf("Search(1, 99) unexpectedly found — columnB is not being compared")
	}
	// A columnA that was never inserted at all.
	if _, found := tree.Search(99, 10); found {
		t.Fatalf("Search(99, 10) unexpectedly found")
	}
}

func TestCompositeOrderingWithSharedColumnA(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	type entry struct{ colA, colB, recordID int64 }
	entries := []entry{
		{5, 300, 1},
		{5, 100, 2},
		{5, 200, 3},
		{3, 500, 4},
		{3, 100, 5},
		{7, 1, 6},
	}
	for _, e := range entries {
		if err := tree.Insert(e.colA, e.colB, e.recordID); err != nil {
			t.Fatalf("Insert(%d, %d) failed: %v", e.colA, e.colB, err)
		}
	}

	for _, e := range entries {
		rid, found := tree.Search(e.colA, e.colB)
		if !found {
			t.Fatalf("Search(%d, %d) not found", e.colA, e.colB)
		}
		if rid != e.recordID {
			t.Fatalf("Search(%d, %d) = %d, want %d (columnB was likely ignored during insert positioning)", e.colA, e.colB, rid, e.recordID)
		}
	}
}

func TestCompositeInsertTriggersSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	const numGroups = 50
	const perGroup = 3 // columnB = 10, 20, 30 within each columnA group

	for a := int64(0); a < numGroups; a++ {
		for i := int64(1); i <= perGroup; i++ {
			b := i * 10
			recordID := a*1000 + b
			if err := tree.Insert(a, b, recordID); err != nil {
				t.Fatalf("Insert(%d, %d) failed: %v", a, b, err)
			}
		}
	}

	for a := int64(0); a < numGroups; a++ {
		for i := int64(1); i <= perGroup; i++ {
			b := i * 10
			want := a*1000 + b
			got, found := tree.Search(a, b)
			if !found {
				t.Fatalf("Search(%d, %d) not found after splits", a, b)
			}
			if got != want {
				t.Fatalf("Search(%d, %d) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestCompositeDeleteTriggersUnderflowHandling(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	const numGroups = 60
	const perGroup = 3

	type key struct{ a, b int64 }
	var allKeys []key
	for a := int64(0); a < numGroups; a++ {
		for i := int64(1); i <= perGroup; i++ {
			b := i * 10
			if err := tree.Insert(a, b, a*1000+b); err != nil {
				t.Fatalf("Insert(%d, %d) failed: %v", a, b, err)
			}
			allKeys = append(allKeys, key{a, b})
		}
	}

	// Delete most entries, leaving a handful — enough deletions to
	// force merges/redistributions to cascade, not just underflow a
	// single leaf.
	deleteCount := len(allKeys) - 10
	for i := 0; i < deleteCount; i++ {
		k := allKeys[i]
		if err := tree.Delete(k.a, k.b); err != nil {
			t.Fatalf("Delete(%d, %d) failed: %v", k.a, k.b, err)
		}
	}

	for i := 0; i < deleteCount; i++ {
		k := allKeys[i]
		if _, found := tree.Search(k.a, k.b); found {
			t.Fatalf("Search(%d, %d) found after delete", k.a, k.b)
		}
	}

	for i := deleteCount; i < len(allKeys); i++ {
		k := allKeys[i]
		want := k.a*1000 + k.b
		got, found := tree.Search(k.a, k.b)
		if !found {
			t.Fatalf("Search(%d, %d) not found after unrelated deletes", k.a, k.b)
		}
		if got != want {
			t.Fatalf("Search(%d, %d) = %d, want %d", k.a, k.b, got, want)
		}
	}
}

func TestCompositeScanFixedColumnA(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	const numGroups = 40
	const perGroup = 20 // columnB = 10, 20, ..., 200

	for i := int64(1); i <= perGroup; i++ {
		for a := int64(0); a < numGroups; a++ {
			b := i * 10
			if err := tree.Insert(a, b, a*10000+b); err != nil {
				t.Fatalf("Insert(%d, %d) failed: %v", a, b, err)
			}
		}
	}

	targetA := int64(17)
	results, err := tree.Scan(targetA, 50, 150)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	var wantB []int64
	for i := int64(1); i <= perGroup; i++ {
		b := i * 10
		if b >= 50 && b <= 150 {
			wantB = append(wantB, b)
		}
	}

	if len(results) != len(wantB) {
		t.Fatalf("Scan(%d, 50, 150) returned %d results, want %d", targetA, len(results), len(wantB))
	}
	for i, r := range results {
		if r.ColumnA != targetA {
			t.Fatalf("Scan(%d, ...)[%d].ColumnA = %d, want %d (leaked another columnA group)", targetA, i, r.ColumnA, targetA)
		}
		if r.ColumnB != wantB[i] {
			t.Fatalf("Scan(%d, ...)[%d].ColumnB = %d, want %d", targetA, i, r.ColumnB, wantB[i])
		}
		wantRID := targetA*10000 + r.ColumnB
		if r.RecordID != wantRID {
			t.Fatalf("Scan(%d, ...)[%d].RecordID = %d, want %d", targetA, i, r.RecordID, wantRID)
		}
	}
}

func TestCompositeScanInvalidRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	if _, err := tree.Scan(1, 100, 10); err != ErrInvalidRange {
		t.Fatalf("Scan with startColumnB > endColumnB error = %v, want ErrInvalidRange", err)
	}
}

func TestCompositeBTreeSurvivesReopenAfterSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree1, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}

	const numGroups = 60
	const perGroup = 3
	type key struct{ a, b, recordID int64 }
	var all []key
	for a := int64(0); a < numGroups; a++ {
		for i := int64(1); i <= perGroup; i++ {
			b := i * 10
			rid := a*1000 + b
			if err := tree1.Insert(a, b, rid); err != nil {
				t.Fatalf("Insert(%d, %d) failed: %v", a, b, err)
			}
			all = append(all, key{a, b, rid})
		}
	}

	if err := tree1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tree2, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer tree2.Close()

	for _, k := range all {
		got, found := tree2.Search(k.a, k.b)
		if !found {
			t.Fatalf("Search(%d, %d) not found after reopen", k.a, k.b)
		}
		if got != k.recordID {
			t.Fatalf("Search(%d, %d) after reopen = %d, want %d", k.a, k.b, got, k.recordID)
		}
	}

	if err := tree2.Insert(999, 1, 12345); err != nil {
		t.Fatalf("Insert after reopen failed: %v", err)
	}
	got, found := tree2.Search(999, 1)
	if !found || got != 12345 {
		t.Fatalf("Search(999, 1) after reopen+insert = (%d, %v), want (12345, true)", got, found)
	}
}

func TestCompositeInsertDuplicateKeyDifferentColumnB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenComposite(dbPath)
	if err != nil {
		t.Fatalf("OpenComposite failed: %v", err)
	}
	defer tree.Close()

	if err := tree.Insert(1, 1, 111); err != nil {
		t.Fatalf("Insert(1,1) failed: %v", err)
	}
	if err := tree.Insert(1, 2, 222); err != nil {
		t.Fatalf("Insert(1,2) failed: %v", err)
	}

	got1, found1 := tree.Search(1, 1)
	got2, found2 := tree.Search(1, 2)
	if !found1 || got1 != 111 {
		t.Fatalf("Search(1,1) = (%d, %v), want (111, true)", got1, found1)
	}
	if !found2 || got2 != 222 {
		t.Fatalf("Search(1,2) = (%d, %v), want (222, true)", got2, found2)
	}
}
