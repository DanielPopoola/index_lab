// Package page implements fixed-size page storage: the on-disk byte layout
// that everything else (buffer pool, B+ tree) is built on top of.
package page

import "encoding/binary"

const (
	// PageSize is the fixed size of every page on disk, per the spec (4 KiB).
	PageSize = 4096

	// HeaderSize is how many bytes at the start of the page are reserved
	// for the header.
	HeaderSize = 21

	// SlotSize is the fixed size, in bytes, of one slot entry in the slot
	// array.
	SlotSize = 4
)

// PageType distinguishes a leaf page (holds real entries) from an internal
// page (holds separator keys + child PageIDs). Only Leaf is needed for now.
type PageType uint8

const (
	LeafPage PageType = iota
	InternalPage
)

// PageID identifies a page's position in the file. PageID N lives at byte
// offset N * PageSize in the underlying file — it is NOT stored as part of
// the page's own bytes; it's derived from where you found the page.
type PageID uint64

// Page is the in-memory representation of one fixed-size page.
// Data is always exactly PageSize bytes — this is the raw buffer that gets
// written to / read from disk verbatim.
type Page struct {
	ID   PageID
	Data [PageSize]byte
}

func NewLeafPage(id PageID) *Page {
	p := &Page{ID: id}
	p.SetPageType(LeafPage)
	p.setFreeSpaceOffset(PageSize)
	return p
}

// --- Header accessors ---
//
// These read/write fixed fields at fixed offsets in Data[0:HeaderSize].

// PageType returns the page's type from the header.
func (p *Page) PageType() PageType {
	return PageType(p.Data[0])
}

// SetPageType writes the page's type into the header.
func (p *Page) SetPageType(t PageType) {
	p.Data[0] = byte(t)
}

// NumEntries returns how many entries (slots) are currently on this page.
func (p *Page) NumEntries() uint16 {
	return binary.BigEndian.Uint16(p.Data[1:3])
}

// setNumEntries writes the entry count into the header.
func (p *Page) setNumEntries(n uint16) {
	binary.BigEndian.PutUint16(p.Data[1:3], n)
}

// NextLeafPageID / PrevLeafPageID: sibling links for leaf pages (spec section 5).
func (p *Page) NextLeafPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[13:21]))
}

func (p *Page) SetNextLeafPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[13:21], uint64(id))
}

func (p *Page) PrevLeafPageID() PageID {
	return PageID(binary.BigEndian.Uint64(p.Data[5:13]))
}

func (p *Page) SetPrevLeafPageID(id PageID) {
	binary.BigEndian.PutUint64(p.Data[5:13], uint64(id))
}

// freeSpaceOffset returns the byte offset where entry data currently starts
func (p *Page) freeSpaceOffset() uint16 {
	return binary.BigEndian.Uint16(p.Data[3:5])
}

func (p *Page) setFreeSpaceOffset(offset uint16) {
	binary.BigEndian.PutUint16(p.Data[3:5], offset)
}

// --- Slot array access ---

// slot is the in-memory decoded form of one slot array entry.
// It is NOT stored as a Go struct on the page — it's decoded from
// SlotSize raw bytes at slot index i, and encoded back the same way.
type slot struct {
	offset uint16 // byte offset into Data where this entry's bytes start
	length uint16 // how many bytes this entry occupies
}

// getSlot decodes the slot at index i from the slot array region of Data.
func (p *Page) getSlot(i uint16) slot {
	start := HeaderSize + i*SlotSize
	offset := binary.BigEndian.Uint16(p.Data[start : start+2])
	length := binary.BigEndian.Uint16(p.Data[start+2 : start+4])
	return slot{offset: offset, length: length}
}

// setSlot encodes a slot's {offset, length} at index i in the slot array.
func (p *Page) setSlot(i uint16, s slot) {
	start := HeaderSize + i*SlotSize
	binary.BigEndian.PutUint16(p.Data[start:start+2], s.offset)
	binary.BigEndian.PutUint16(p.Data[start+2:start+4], s.length)
}

// --- Entry read/write ---
//
// An "entry" here is the already-encoded bytes for one (key, value) pair.

// GetEntry returns the raw bytes of the entry at the given slot index.
func (p *Page) GetEntry(i uint16) []byte {
	slot := p.getSlot(i)
	return p.Data[slot.offset : slot.offset+slot.length]
}

// HasSpaceFor reports whether entryLen bytes of new entry data would fit
// in the page's current free space, ALSO accounting for the new slot
// (SlotSize bytes) that would need to be added to the slot array.
func (p *Page) HasSpaceFor(entryLen int) bool {
	left := HeaderSize + p.NumEntries()*SlotSize
	right := p.freeSpaceOffset()

	available := int(right) - int(left)
	return available >= int(SlotSize)+entryLen
}

// InsertEntry inserts entryBytes as a new entry, placing its slot at
// sorted position `slotIndex` in the slot array..
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

// DeleteEntry removes the entry at the given slot index, shifting later

// slots left by one. Does NOT reclaim the physical bytes of the deleted
// entry in the entry-data area (that's fragmentation — out of scope here,
// per spec section 18; real databases fix this via periodic compaction).
func (p *Page) DeleteEntry(slotIndex uint16) {
	n := p.NumEntries()

	for i := slotIndex; i < n-1; i++ {
		moving := p.getSlot(i + 1)
		p.setSlot(i, moving)
	}

	p.setNumEntries(n - 1)
}
