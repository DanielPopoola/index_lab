// Package partial implements a partial index: a B+ tree that only ever
// contains entries for rows satisfying one fixed predicate,
// Status == Active. The underlying *btree.BTree never knows this
// predicate exists — it just receives Insert/Delete calls, or doesn't,
// based on a decision made entirely in this package.
package partial

import (
	"github.com/DanielPopoola/index_lab/internal/btree"
)

// Status is a row's indexing eligibility. Active rows belong in the
// index; every other status does not. This project implements exactly
// one fixed predicate (Status == Active) — not general user-defined
// predicates — so Status only needs two values to demonstrate it.
type Status int

const (
	Deleted Status = iota
	Active
)

// Row is the minimal row model the spec asks for: a key, a RecordID
// it maps to, and the status that decides whether it belongs in the
// index right now.
type Row struct {
	Key      int64
	RecordID int64
	Status   Status
}

// PartialIndex wraps a *btree.BTree, keeping it in sync with each
// row's Status: present in the tree only while Status == Active.
type PartialIndex struct {
	tree *btree.BTree
}

// Open opens or creates the partial index's underlying B+ tree at path.
func Open(path string) (*PartialIndex, error) {
	tree, err := btree.Open(path)
	if err != nil {
		return nil, err
	}
	return &PartialIndex{tree: tree}, nil
}

// Close closes the underlying tree.
func (pi *PartialIndex) Close() error {
	return pi.tree.Close()
}

// Upsert reconciles the index against row's current Status:
//   - Active and not yet indexed:      insert it.
//   - Active and already indexed:      update the stored RecordID
//     (the row's data may have changed even though its key/status
//     didn't — Delete then Insert is the simplest correct way to
//     apply that, since BTree has no in-place update).
//   - Not Active and currently indexed: remove it.
//   - Not Active and not indexed:       nothing to do.
func (pi *PartialIndex) Upsert(row Row) error {
	_, alreadyIndexed := pi.tree.Search(row.Key)

	switch {
	case row.Status == Active && !alreadyIndexed:
		return pi.tree.Insert(row.Key, row.RecordID)

	case row.Status == Active && alreadyIndexed:
		if err := pi.tree.Delete(row.Key); err != nil {
			return err
		}
		return pi.tree.Insert(row.Key, row.RecordID)

	case row.Status != Active && alreadyIndexed:
		return pi.tree.Delete(row.Key)

	default: // row.Status != Active && !alreadyIndexed
		return nil
	}
}

// Search looks up key in the partial index. Only ever finds it if the
// row was Active the last time Upsert was called for that key.
func (pi *PartialIndex) Search(key int64) (recordID int64, found bool) {
	return pi.tree.Search(key)
}

// Scan returns every indexed (i.e. currently Active) entry with
// startKey <= key <= endKey, in ascending key order.
func (pi *PartialIndex) Scan(startKey, endKey int64) ([]btree.ScanResult, error) {
	return pi.tree.Scan(startKey, endKey)
}
