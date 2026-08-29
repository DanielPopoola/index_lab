package btree

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/DanielPopoola/index_lab/internal/page"
)

func TestBTreeSurvivesReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	entries := []struct{ key, recordID int64 }{
		{30, 300},
		{10, 100},
		{20, 200},
	}

	tree1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for _, e := range entries {
		if err := tree1.Insert(e.key, e.recordID); err != nil {
			t.Fatalf("Insert(%d) failed: %v", e.key, err)
		}
	}

	if err := tree1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tree2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree2.Close()

	for _, e := range entries {
		expectedRecordID, found := tree2.Search(e.key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", e.key)
			continue
		}
		if expectedRecordID != e.recordID {
			t.Errorf("Search(%d): recordID = %d, want %d", e.key, expectedRecordID, e.recordID)
		}
	}
}

func TestBTreeSurvivesReopenAfterSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	const numKeys = 300 // comfortably past the ~203-entry-per-page split threshold

	tree1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for i := int64(0); i < numKeys; i++ {
		if err := tree1.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootBeforeClose := tree1.rootID
	if rootBeforeClose == 1 {
		t.Fatalf("setup problem: root is still PageID 1 after %d inserts, expected a split to have changed it", numKeys)
	}

	if err := tree1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tree2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree2.Close()

	if tree2.rootID != rootBeforeClose {
		t.Fatalf("root PageID after reopen = %d, want %d (the root from before closing)", tree2.rootID, rootBeforeClose)
	}

	for _, key := range []int64{0, 1, numKeys / 2, numKeys - 1} {
		gotRecordID, found := tree2.Search(key)
		if !found {
			t.Errorf("Search(%d): after reopen, expected found=true, got false", key)
			continue
		}
		if gotRecordID != key*10 {
			t.Errorf("Search(%d): after reopen, recordID = %d, want %d", key, gotRecordID, key*10)
		}
	}
}

func TestInsertTriggersSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 220

	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("expected root to be InternalPage after %d inserts (a split should have happened), got PageType=%v", numKeys, rootPage.PageType())
	}

	for i := int64(0); i < numKeys; i++ {
		gotRecordID, found := tree.Search(i)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", i)
			continue
		}
		if gotRecordID != i*10 {
			t.Errorf("Search(%d): recordID = %d, want %d", i, gotRecordID, i*10)
		}
	}

	if _, found := tree.Search(99999); found {
		t.Errorf("Search(99999): expected found=false, got true")
	}
}

func TestInsertTriggersMultiLevelSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 45000

	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("expected root to be InternalPage after %d inserts, got PageType=%v", numKeys, rootPage.PageType())
	}

	leftmostChild, err := tree.pm.ReadPage(rootPage.LeftmostChildPageID())
	if err != nil {
		t.Fatalf("ReadPage(root's leftmost child) failed: %v", err)
	}
	if leftmostChild.PageType() != page.InternalPage {
		t.Fatalf("expected root's leftmost child to be InternalPage (proof of a second split level), got PageType=%v", leftmostChild.PageType())
	}

	spotChecks := []int64{0, 1, numKeys / 4, numKeys / 2, (3 * numKeys) / 4, numKeys - 1}
	for _, key := range spotChecks {
		gotRecordID, found := tree.Search(key)
		if !found {
			t.Errorf("Search(%d): expected found=true, got false", key)
			continue
		}
		if gotRecordID != key*10 {
			t.Errorf("Search(%d): recordID = %d, want %d", key, gotRecordID, key*10)
		}
	}

	if _, found := tree.Search(numKeys + 999); found {
		t.Errorf("Search(%d): expected found=false, got true", numKeys+999)
	}
}

func TestDeleteTriggersMultiLevelCascade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 45000

	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("setup problem: expected root to be InternalPage after %d inserts, got PageType=%v", numKeys, rootPage.PageType())
	}
	leftmostChild, err := tree.pm.ReadPage(rootPage.LeftmostChildPageID())
	if err != nil {
		t.Fatalf("ReadPage(root's leftmost child) failed: %v", err)
	}
	if leftmostChild.PageType() != page.InternalPage {
		t.Fatalf("setup problem: expected root's leftmost child to be InternalPage (3-level tree), got PageType=%v", leftmostChild.PageType())
	}

	const deleteUpTo = 20000 // delete keys [0, deleteUpTo)

	for i := int64(0); i < deleteUpTo; i++ {
		if err := tree.Delete(i); err != nil {
			t.Fatalf("Delete(%d) failed: %v", i, err)
		}
	}

	// Deleted keys must all be gone.
	deletedSpotChecks := []int64{0, 1, deleteUpTo / 4, deleteUpTo / 2, deleteUpTo - 1}
	for _, key := range deletedSpotChecks {
		if _, found := tree.Search(key); found {
			t.Errorf("Search(%d): expected found=false after delete, got true", key)
		}
	}

	survivingSpotChecks := []int64{
		deleteUpTo,
		deleteUpTo + 1,
		deleteUpTo + (numKeys-deleteUpTo)/4,
		(deleteUpTo + numKeys) / 2,
		numKeys - 1,
	}
	for _, key := range survivingSpotChecks {
		gotRecordID, found := tree.Search(key)
		if !found {
			t.Errorf("Search(%d): expected found=true (surviving key), got false", key)
			continue
		}
		if gotRecordID != key*10 {
			t.Errorf("Search(%d): recordID = %d, want %d", key, gotRecordID, key*10)
		}
	}

	if err := tree.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (reopen) failed: %v", err)
	}
	defer reopened.Close()

	if _, found := reopened.Search(deleteUpTo / 2); found {
		t.Errorf("Search(%d) after reopen: expected found=false, got true", deleteUpTo/2)
	}
	gotRecordID, found := reopened.Search(numKeys - 1)
	if !found {
		t.Errorf("Search(%d) after reopen: expected found=true, got false", numKeys-1)
	} else if gotRecordID != (numKeys-1)*10 {
		t.Errorf("Search(%d) after reopen: recordID = %d, want %d", numKeys-1, gotRecordID, (numKeys-1)*10)
	}
}

func TestLeafChainCorrectAfterNonSequentialInserts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 5000

	keys := make([]int64, numKeys)
	for i := range keys {
		keys[i] = int64(i)
	}
	rand.Shuffle(len(keys), func(i, j int) {
		keys[i], keys[j] = keys[j], keys[i]
	})

	for _, k := range keys {
		if err := tree.Insert(k, k*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", k, err)
		}
	}

	// Confirm this actually built a multi-page tree — otherwise the
	// test isn't exercising anything interesting.
	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("setup problem: expected root to be InternalPage after %d shuffled inserts, got PageType=%v", numKeys, rootPage.PageType())
	}

	// Walk down the tree's leftmost spine to find the leftmost leaf —
	// the correct starting point for a forward walk of the leaf chain.
	current := rootPage
	for current.PageType() != page.LeafPage {
		current, err = tree.pm.ReadPage(current.LeftmostChildPageID())
		if err != nil {
			t.Fatalf("ReadPage while descending to leftmost leaf failed: %v", err)
		}
	}
	leftmostLeaf := current

	// Forward walk: collect every key from every leaf via
	// NextLeafPageID, and confirm strictly ascending order end to end
	// — the real proof that splits wired the chain correctly
	// regardless of where in the tree they happened.
	var gotKeys []int64
	leaf := leftmostLeaf
	for {
		for i := uint16(0); i < leaf.NumEntries(); i++ {
			entry := leaf.GetEntry(i)
			key := DecodeInt64(entry[:8])
			gotKeys = append(gotKeys, key)
		}
		if leaf.NextLeafPageID() == 0 {
			break
		}
		leaf, err = tree.pm.ReadPage(leaf.NextLeafPageID())
		if err != nil {
			t.Fatalf("ReadPage while walking leaf chain forward failed: %v", err)
		}
	}

	if len(gotKeys) != numKeys {
		t.Fatalf("leaf chain walk found %d keys, want %d", len(gotKeys), numKeys)
	}
	for i := 1; i < len(gotKeys); i++ {
		if gotKeys[i] <= gotKeys[i-1] {
			t.Fatalf("leaf chain out of order at position %d: %d <= %d", i, gotKeys[i], gotKeys[i-1])
		}
	}
	if gotKeys[0] != 0 || gotKeys[len(gotKeys)-1] != numKeys-1 {
		t.Fatalf("leaf chain endpoints wrong: first=%d last=%d, want first=0 last=%d", gotKeys[0], gotKeys[len(gotKeys)-1], numKeys-1)
	}

	// Backward walk: starting from the LAST leaf reached above,
	// follow PrevLeafPageID back to the start. If any leaf's Prev
	// pointer is stale or wrong, this walk either breaks early,
	// loops, or lands somewhere other than leftmostLeaf.
	var backKeys []int64
	back := leaf // the last leaf from the forward walk (NextLeafPageID == 0)
	visited := make(map[page.PageID]bool)
	for {
		if visited[back.ID] {
			t.Fatalf("backward walk revisited PageID %d — Prev pointers form a cycle", back.ID)
		}
		visited[back.ID] = true

		for i := int(back.NumEntries()) - 1; i >= 0; i-- {
			entry := back.GetEntry(uint16(i))
			key := DecodeInt64(entry[:8])
			backKeys = append(backKeys, key)
		}
		if back.PrevLeafPageID() == 0 {
			break
		}
		back, err = tree.pm.ReadPage(back.PrevLeafPageID())
		if err != nil {
			t.Fatalf("ReadPage while walking leaf chain backward failed: %v", err)
		}
	}

	if back.ID != leftmostLeaf.ID {
		t.Fatalf("backward walk ended at PageID %d, want %d (leftmostLeaf) — Prev chain is broken", back.ID, leftmostLeaf.ID)
	}
	if len(backKeys) != numKeys {
		t.Fatalf("backward walk found %d keys, want %d", len(backKeys), numKeys)
	}
	// backKeys was collected leaf-by-leaf in reverse, each leaf's own
	// entries also read in reverse — so backKeys should be gotKeys
	// exactly reversed.
	for i := range backKeys {
		want := gotKeys[len(gotKeys)-1-i]
		if backKeys[i] != want {
			t.Fatalf("backward walk key mismatch at position %d: got %d, want %d", i, backKeys[i], want)
		}
	}
}

func TestUniqueIndexRejectsDuplicate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenUnique(dbPath)
	if err != nil {
		t.Fatalf("OpenUnique failed: %v", err)
	}
	defer tree.Close()

	if err := tree.Insert(42, 100); err != nil {
		t.Fatalf("first Insert(42) failed: %v", err)
	}

	err = tree.Insert(42, 999)
	if err != ErrDuplicateKey {
		t.Fatalf("second Insert(42) error = %v, want ErrDuplicateKey", err)
	}

	// The original value must be untouched — a rejected duplicate
	// insert must not have partially overwritten anything.
	got, found := tree.Search(42)
	if !found {
		t.Fatalf("Search(42) not found after rejected duplicate insert")
	}
	if got != 100 {
		t.Fatalf("Search(42) = %d, want 100 (original value should survive a rejected duplicate insert)", got)
	}
}

func TestOrdinaryIndexStillAllowsDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	if err := tree.Insert(7, 111); err != nil {
		t.Fatalf("first Insert(7) failed: %v", err)
	}
	if err := tree.Insert(7, 222); err != nil {
		t.Fatalf("second Insert(7) on a non-unique tree failed: %v (should be allowed)", err)
	}
}

func TestUniqueIndexRejectsDuplicateAcrossLeafSplit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := OpenUnique(dbPath)
	if err != nil {
		t.Fatalf("OpenUnique failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 500
	for i := int64(0); i < numKeys; i++ {
		if err := tree.Insert(i, i*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", i, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("setup problem: expected a multi-page tree after %d inserts, got a single leaf root", numKeys)
	}

	// Try re-inserting a key from early in the range, the middle, and
	// the end — different leaves post-split.
	for _, dup := range []int64{0, numKeys / 2, numKeys - 1} {
		err := tree.Insert(dup, 99999)
		if err != ErrDuplicateKey {
			t.Fatalf("Insert(%d) [duplicate, post-split] error = %v, want ErrDuplicateKey", dup, err)
		}
		got, found := tree.Search(dup)
		if !found || got != dup*10 {
			t.Fatalf("Search(%d) after rejected duplicate = (%d, %v), want (%d, true)", dup, got, found, dup*10)
		}
	}

	// A genuinely new key must still insert successfully — the
	// uniqueness check shouldn't be rejecting everything.
	if err := tree.Insert(numKeys+1, 12345); err != nil {
		t.Fatalf("Insert of a genuinely new key failed: %v", err)
	}
}

func TestScan(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	tree, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer tree.Close()

	const numKeys = 500
	for i := int64(0); i < numKeys; i++ {
		key := i * 2
		if err := tree.Insert(key, key*10); err != nil {
			t.Fatalf("Insert(%d) failed: %v", key, err)
		}
	}

	rootPage, err := tree.pm.ReadPage(tree.rootID)
	if err != nil {
		t.Fatalf("ReadPage(root) failed: %v", err)
	}
	if rootPage.PageType() != page.InternalPage {
		t.Fatalf("setup problem: expected a multi-page tree after %d inserts, got a single leaf root", numKeys)
	}

	// wantKeys returns every even key in [lo, hi], inclusive — the
	// ground truth to compare Scan's output against.
	wantKeys := func(lo, hi int64) []int64 {
		var want []int64
		for k := int64(0); k < numKeys*2; k += 2 {
			if k >= lo && k <= hi {
				want = append(want, k)
			}
		}
		return want
	}

	checkScan := func(t *testing.T, startKey, endKey int64) {
		t.Helper()
		results, err := tree.Scan(startKey, endKey)
		if err != nil {
			t.Fatalf("Scan(%d, %d) failed: %v", startKey, endKey, err)
		}

		want := wantKeys(startKey, endKey)
		if len(results) != len(want) {
			t.Fatalf("Scan(%d, %d) returned %d results, want %d", startKey, endKey, len(results), len(want))
		}
		for i, r := range results {
			if r.Key != want[i] {
				t.Fatalf("Scan(%d, %d)[%d].Key = %d, want %d", startKey, endKey, i, r.Key, want[i])
			}
			if r.RecordID != r.Key*10 {
				t.Errorf("Scan(%d, %d)[%d].RecordID = %d, want %d", startKey, endKey, i, r.RecordID, r.Key*10)
			}
		}
	}

	t.Run("mid-chain range spanning a leaf split", func(t *testing.T) {
		// Comfortably inside the key space, wide enough to cross
		// several leaf boundaries.
		checkScan(t, 100, 300)
	})

	t.Run("range start and end fall between stored keys", func(t *testing.T) {
		// Keys are all even; 101 and 299 don't exactly match any
		// entry, exercising findKeyIndex's "insertion point" path
		// and the key > endKey overshoot check.
		checkScan(t, 101, 299)
	})

	t.Run("range starts before every key in the tree", func(t *testing.T) {
		checkScan(t, -1000, 50)
	})

	t.Run("range ends after every key in the tree", func(t *testing.T) {
		checkScan(t, numKeys*2-50, numKeys*2+1000)
	})

	t.Run("range covers the entire tree", func(t *testing.T) {
		checkScan(t, -1000, numKeys*2+1000)
	})

	t.Run("single-point range on an existing key", func(t *testing.T) {
		checkScan(t, 200, 200)
	})

	t.Run("single-point range on a missing key", func(t *testing.T) {
		checkScan(t, 201, 201) // odd — never inserted
	})

	t.Run("range entirely outside the tree, above", func(t *testing.T) {
		checkScan(t, numKeys*2+100, numKeys*2+200)
	})

	t.Run("range entirely outside the tree, below", func(t *testing.T) {
		checkScan(t, -500, -100)
	})

	t.Run("invalid range returns ErrInvalidRange", func(t *testing.T) {
		results, err := tree.Scan(50, 10)
		if err != ErrInvalidRange {
			t.Fatalf("Scan(50, 10) error = %v, want ErrInvalidRange", err)
		}
		if results != nil {
			t.Fatalf("Scan(50, 10) results = %v, want nil", results)
		}
	})
}
