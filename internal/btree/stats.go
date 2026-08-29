package btree

import (
	"encoding/binary"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// TreeStats reports the tree's current shape and cumulative event
// counts. Structural fields (Height, page/entry counts) are computed
// fresh by walking the real tree every time Stats is called — there's
// no incremental bookkeeping to keep in sync with every split, merge,
// and root-shrink, so they can't silently drift from what's actually
// on disk. Event fields (splits, merges, reads, writes) are running
// counters incremented at the source as those operations happen.
type TreeStats struct {
	Height        int
	TotalPages    int
	LeafPages     int
	InternalPages int
	TotalEntries  int
	PageSplits    uint64
	PageMerges    uint64
	PageReads     uint64
	PageWrites    uint64
}

// Stats walks the tree from the root and reports its current shape
// alongside the running event counters.
func (t *BTree) Stats() (TreeStats, error) {
	stats := TreeStats{
		PageSplits: t.pageSplits,
		PageMerges: t.pageMerges,
		PageReads:  t.pm.PageReads(),
		PageWrites: t.pm.PageWrites(),
	}

	root, err := t.pm.ReadPage(t.rootID)
	if err != nil {
		return TreeStats{}, err
	}

	// BFS, level by level, so Height falls out naturally as the
	// number of levels visited before hitting leaves — no separate
	// depth-tracking needed beyond "how many times did this loop run."
	level := []*page.Page{root}
	for len(level) > 0 {
		stats.Height++

		var nextLevel []*page.Page
		for _, p := range level {
			stats.TotalPages++

			if p.PageType() == page.LeafPage {
				stats.TotalEntries += int(p.NumEntries())
				stats.LeafPages++
				continue
			}

			stats.InternalPages++
			children, err := childPageIDs(p)
			if err != nil {
				return TreeStats{}, err
			}
			for _, childID := range children {
				child, err := t.pm.ReadPage(childID)
				if err != nil {
					return TreeStats{}, err
				}
				nextLevel = append(nextLevel, child)
			}
		}
		level = nextLevel
	}

	return stats, nil
}

// childPageIDs returns every child PageID an internal page points to:
// LeftmostChildPageID plus one child per entry. Not meaningful for
// leaf pages — callers must check PageType first.
func childPageIDs(p *page.Page) ([]page.PageID, error) {
	keyLen := entryKeyLen(p)

	ids := []page.PageID{p.LeftmostChildPageID()}
	for i := uint16(0); i < p.NumEntries(); i++ {
		entry := p.GetEntry(i)
		childBytes := entry[keyLen:]
		ids = append(ids, page.PageID(binary.BigEndian.Uint64(childBytes)))
	}
	return ids, nil
}

// ResetStats zeroes every counter (splits, merges, reads, writes) for
// a fresh measurement window. Structural stats need no resetting —
// Stats() always recomputes them from the tree's actual current shape.
func (t *BTree) ResetStats() {
	t.pageSplits = 0
	t.pageMerges = 0
	t.pm.ResetStats()
}
