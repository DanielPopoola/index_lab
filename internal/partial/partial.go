// Package partial implements a partial index: a B+ tree that only ever
// contains entries for rows satisfying one fixed predicate,
// Status == Active. The underlying *btree.BTree never knows this
// predicate exists — it just receives Insert/Delete calls, or doesn't,
// based on a decision made entirely in this package.
package partial

import (
	"github.com/DanielPopoola/index_lab/internal/btree"
)

// Status indicates whether a row should be indexed (Active) or not.
type Status int

const (
	Deleted Status = iota
	Active
)

// Row represents a row with its key, RecordID, and indexing status.
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

// Stats reports the underlying tree's current shape and event counters.
func (pi *PartialIndex) Stats() (btree.TreeStats, error) {
	return pi.tree.Stats()
}

// ResetStats zeroes the underlying tree's event counters.
func (pi *PartialIndex) ResetStats() {
	pi.tree.ResetStats()
}

// Upsert reconciles the index against the row's current Status:
// inserting if Active and missing, deleting if not Active and present,
// or updating RecordID if Active and already indexed.
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
