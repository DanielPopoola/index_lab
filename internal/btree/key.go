// Package btree will eventually hold the B+ tree itself (tree.go, node.go).
// key.go specifically handles turning a Go int64 into a byte representation
// that sorts correctly under plain byte comparison — this is what lets the
// page package's slot array stay sorted using nothing but []byte comparison,
// with no knowledge of what a "key" actually means.
package btree

import (
	"bytes"
	"encoding/binary"
)

// signBitMask has only the most-significant bit of a uint64 set (binary:
// 1000...0, 63 zeros after the 1). XORing any uint64 against this flips
// exactly that one bit and leaves the other 63 untouched.
const signBitMask = uint64(1) << 63

func EncodeInt64(n int64) []byte {
	bits := uint64(n)
	flipped := bits ^ signBitMask

	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, flipped)
	return encoded
}

func DecodeInt64(encoded []byte) int64 {
	decoded := binary.BigEndian.Uint64(encoded)
	flipped := decoded ^ signBitMask
	return int64(flipped)
}

// CompareKeys compares two already-encoded keys and reports their order:
// -1 if a < b, 0 if equal, 1 if a > b. Pure byte comparison — no decoding
// needed, because EncodeInt64 already guarantees byte order matches value
// order.
func CompareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}
