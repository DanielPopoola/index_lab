package btree

import (
	"bytes"
	"encoding/binary"
)

// signBitMask flips the sign bit to make int64 values comparable as uint64.
const signBitMask = uint64(1) << 63

// EncodeInt64 encodes a signed int64 into 8 bytes for comparison.
func EncodeInt64(n int64) []byte {
	bits := uint64(n)
	flipped := bits ^ signBitMask

	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, flipped)
	return encoded
}

// DecodeInt64 decodes a signed int64 from encoded bytes.
func DecodeInt64(encoded []byte) int64 {
	decoded := binary.BigEndian.Uint64(encoded)
	flipped := decoded ^ signBitMask
	return int64(flipped)
}

// CompareKeys compares two encoded keys.
func CompareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}

// CompositeEntrySize is the total per-entry byte width for a composite key.
const CompositeEntrySize = 24

// EncodeCompositeKey encodes two int64 columns into a composite key.
func EncodeCompositeKey(columnA, columnB int64) []byte {
	encodedColumnA := EncodeInt64(columnA)
	encodedColumnB := EncodeInt64(columnB)
	return append(append([]byte(nil), encodedColumnA...), encodedColumnB...)
}

// DecodeCompositeKey decodes a composite key into two int64 columns.
func DecodeCompositeKey(encoded []byte) (columnA, columnB int64) {
	encodedColumnA := encoded[:8]
	encodedColumnB := encoded[8:]
	return DecodeInt64(encodedColumnA), DecodeInt64(encodedColumnB)
}
