package btree

import (
	"encoding/binary"

	"github.com/DanielPopoola/index_lab/internal/page"
)

// TreeStats reports the tree's current shape and cumulative event counts.
// Structural fields (Height, page/entry counts) are computed by walking
// the tree; event fields are running counters incremented as operations happen.
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

	// BFS level by level to compute Height and gather page stats.
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

// childPageIDs returns the child PageIDs of an internal page.
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

// ResetStats zeroes all event counters for a fresh measurement window.
func (t *BTree) ResetStats() {
	t.pageSplits = 0
	t.pageMerges = 0
	t.pm.ResetStats()
}
