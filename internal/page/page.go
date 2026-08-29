// Package page implements fixed-size page storage: the on-disk byte layout
// that everything else (buffer pool, B+ tree) is built on top of.
package page

import "encoding/binary"

const (
	PageSize   = 4096 // fixed size of every page on disk, in bytes
	HeaderSize = 21   // bytes reserved at the start of every page for the header
	SlotSize   = 4    // bytes per slot array entry
	EntrySize  = 16   // bytes per entry: 8-byte key + 8-byte value/childID (true for both leaf and internal entries)
)

type PageType uint8

const (
	LeafPage PageType = iota
	InternalPage
	MetadataPage
	HeapPage
)

// PageID identifies a page's position in a file.
type PageID uint64

// Page represents an in-memory fixed-size page.
type Page struct {
	ID   PageID
	Data [PageSize]byte
}

// NewLeafPage builds a fresh, empty leaf page with the given ID.
func NewLeafPage(id PageID) *Page {
	p := &Page{ID: id}
	p.SetPageType(LeafPage)
	p.setFreeSpaceOffset(PageSize)
	return p
}

// NewHeapPage builds a fresh, empty heap page with the given ID.
func NewHeapPage(id PageID) *Page {
	p := &Page{ID: id}
	p.SetPageType(HeapPage)
	p.setFreeSpaceOffset(PageSize)
	return p
}

// NewInternalPage builds a fresh, empty internal page with the given ID and leftmost child.
func NewInternalPage(id PageID, leftmostChild PageID) *Page {
	p := &Page{ID: id}
	p.SetPageType(InternalPage)
	p.setFreeSpaceOffset(PageSize)
	p.SetLeftmostChildPageID(leftmostChild)
	return p
}

// NewMetadataPage builds a metadata page holding `rootID`.
func NewMetadataPage(id PageID, rootID PageID) *Page {
	p := &Page{ID: id}
	p.SetPageType(MetadataPage)
	p.SetRootPageID(rootID)
	return p
}

// RootPageID returns the root PageID stored on a metadata page.
func (p *Page) RootPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[1:9]))
}

// SetRootPageID sets the root PageID on a metadata page.
func (p *Page) SetRootPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[1:9], uint64(id))
}

// PageType returns the page's type.
func (p *Page) PageType() PageType {
	return PageType(p.Data[0])
}

// SetPageType sets the page's type.
func (p *Page) SetPageType(t PageType) {
	p.Data[0] = byte(t)
}

// NumEntries returns the current entry count.
func (p *Page) NumEntries() uint16 {
	return binary.BigEndian.Uint16(p.Data[1:3])
}

// setNumEntries writes the entry count into the header.
func (p *Page) setNumEntries(n uint16) {
	binary.BigEndian.PutUint16(p.Data[1:3], n)
}

// NextLeafPageID returns the right-sibling pointer for leaf pages.
func (p *Page) NextLeafPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[13:21]))
}

// SetNextLeafPageID sets the right-sibling pointer.
func (p *Page) SetNextLeafPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[13:21], uint64(id))
}

// PrevLeafPageID returns the left-sibling pointer for leaf pages.
func (p *Page) PrevLeafPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[5:13]))
}

// SetPrevLeafPageID sets the left-sibling pointer.
func (p *Page) SetPrevLeafPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[5:13], uint64(id))
}

// LeftmostChildPageID returns the leftmost-child pointer for internal pages.
func (p *Page) LeftmostChildPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[5:13]))
}

// SetLeftmostChildPageID sets the leftmost-child pointer.
func (p *Page) SetLeftmostChildPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[5:13], uint64(id))
}

// freeSpaceOffset returns the byte offset where entry data currently
// starts, growing downward from PageSize as entries are inserted.
func (p *Page) freeSpaceOffset() uint16 {
	return binary.BigEndian.Uint16(p.Data[3:5])
}

// setFreeSpaceOffset writes the current free-space offset into the
// header.
func (p *Page) setFreeSpaceOffset(offset uint16) {
	binary.BigEndian.PutUint16(p.Data[3:5], offset)
}

// --- Slot array access ---

// slot represents one slot array entry.
type slot struct {
	offset uint16 // byte offset into Data where this entry's bytes start
	length uint16 // how many bytes this entry occupies
}

// getSlot decodes the slot at index i from the slot array.
func (p *Page) getSlot(i uint16) slot {
	start := HeaderSize + i*SlotSize
	offset := binary.BigEndian.Uint16(p.Data[start : start+2])
	length := binary.BigEndian.Uint16(p.Data[start+2 : start+4])
	return slot{offset: offset, length: length}
}

// setSlot encodes a slot at index i in the slot array.
func (p *Page) setSlot(i uint16, s slot) {
	start := HeaderSize + i*SlotSize
	binary.BigEndian.PutUint16(p.Data[start:start+2], s.offset)
	binary.BigEndian.PutUint16(p.Data[start+2:start+4], s.length)
}

// GetEntry returns the raw bytes of the entry at slot index i.
func (p *Page) GetEntry(i uint16) []byte {
	slot := p.getSlot(i)
	return p.Data[slot.offset : slot.offset+slot.length]
}

// HasSpaceFor reports whether entryLen bytes of new entry data would fit.
func (p *Page) HasSpaceFor(entryLen int) bool {
	left := HeaderSize + p.NumEntries()*SlotSize
	right := p.freeSpaceOffset()

	available := int(right) - int(left)
	return available >= int(SlotSize)+entryLen
}

// MaxEntries returns the maximum number of fixed-size entries
func MaxEntries(entrySize uint16) uint16 {
	return (PageSize - HeaderSize) / (SlotSize + entrySize)
}

// MinEntries returns the minimum number of entries a non-root page must
// hold to satisfy the B+ tree's occupancy invariant ("at least half
// full"). The root page is exempt from this rule — callers must check
// for that separately.
func MinEntries(entrySize uint16) uint16 {
	return MaxEntries(entrySize) / 2
}

// InsertEntry inserts entryBytes as a new entry at the sorted position slotIndex.
func (p *Page) InsertEntry(slotIndex uint16, entryBytes []byte) {
	n := p.NumEntries()

	for i := n; i > slotIndex; i-- {
		moving := p.getSlot(i - 1)
		p.setSlot(i, moving)
	}

	newEntryOffset := p.freeSpaceOffset() - uint16(len(entryBytes))

	copy(p.Data[newEntryOffset:newEntryOffset+uint16(len(entryBytes))], entryBytes)

	p.setSlot(slotIndex, slot{
		offset: newEntryOffset,
		length: uint16(len(entryBytes)),
	})

	p.setFreeSpaceOffset(newEntryOffset)
	p.setNumEntries(n + 1)
}

// Removes the entry at `slotIndex` and compacts the remaining entries.
//
// Entry data is stored from the end of the page downward. Merely shifting
// slots would make the deleted entry's bytes unreachable but would not
// reclaim their space, eventually causing merges to run out of physical
// space even though NumEntries is below MaxEntries.
func (p *Page) DeleteEntry(slotIndex uint16) {
	n := p.NumEntries()
	if slotIndex >= n {
		return
	}

	entries := make([][]byte, 0, n-1)
	for i := uint16(0); i < n; i++ {
		if i == slotIndex {
			continue
		}
		entries = append(entries, append([]byte(nil), p.GetEntry(i)...))
	}

	p.setNumEntries(0)
	p.setFreeSpaceOffset(PageSize)
	for i, entry := range entries {
		p.InsertEntry(uint16(i), entry)
	}
}
