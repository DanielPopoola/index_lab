package btree

import (
	"bytes"
	"encoding/binary"
)

// signBitMask has only the most-significant bit of a uint64 set (binary:
// 1000...0, 63 zeros after the 1). XORing any uint64 against this flips
// exactly that one bit and leaves the other 63 untouched.
const signBitMask = uint64(1) << 63

// Encodes a signed `int64` into 8 bytes whose unsigned byte order matches the value's signed numeric order
func EncodeInt64(n int64) []byte {
	bits := uint64(n)
	flipped := bits ^ signBitMask

	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, flipped)
	return encoded
}

// Inverse of `EncodeInt64`.
func DecodeInt64(encoded []byte) int64 {
	decoded := binary.BigEndian.Uint64(encoded)
	flipped := decoded ^ signBitMask
	return int64(flipped)
}

// Compares two already-encoded keys.
func CompareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}

// CompositeEntrySize is the total per-entry byte width for a composite
// (columnA, columnB) index: 16-byte key (two 8-byte columns) + 8-byte
// value/recordID, vs. the single-column tree's 8+8=16.
const CompositeEntrySize = 24

func EncodeCompositeKey(columnA, columnB int64) []byte {
	encodedColumnA := EncodeInt64(columnA)
	encodedColumnB := EncodeInt64(columnB)
	return append(append([]byte(nil), encodedColumnA...), encodedColumnB...)
}

func DecodeCompositeKey(encoded []byte) (columnA, columnB int64) {
	encodedColumnA := encoded[:8]
	encodedColumnB := encoded[8:]
	return DecodeInt64(encodedColumnA), DecodeInt64(encodedColumnB)
}
