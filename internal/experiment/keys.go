// Package experiment provides a small harness for driving btree.BTree
// (and CompositeBTree/heap.Heap) under different workloads and
// reporting on the resulting page-level statistics. It exists to
// demonstrate B+ tree locality and page-splitting behavior under
// different key-generation patterns — not to produce production-grade
// benchmarks (the task spec explicitly rules that out).
package experiment

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// KeyPattern names one of the four key-generation strategies the task
// spec asks for.
type KeyPattern string

const (
	Sequential KeyPattern = "sequential"
	Random     KeyPattern = "random"
	UUID4Like  KeyPattern = "uuid4-like"
	UUID7Like  KeyPattern = "uuid7-like"
)

// GenerateKeys returns n int64 keys following the given pattern.
// Callers must insert keys in generation order; some patterns like
// UUID7Like have increasing high bits that provide good B+ tree locality
// only if inserted sequentially.
func GenerateKeys(pattern KeyPattern, n int) []int64 {
	switch pattern {
	case Sequential:
		return sequentialKeys(n)
	case Random:
		return randomKeys(n)
	case UUID4Like:
		return uuid4LikeKeys(n)
	case UUID7Like:
		return uuid7LikeKeys(n)
	default:
		return nil
	}
}

// sequentialKeys returns 0, 1, 2, ..., n-1.
func sequentialKeys(n int) []int64 {
	keys := make([]int64, n)
	for i := range keys {
		keys[i] = int64(i)
	}
	return keys
}

// randomKeys returns n uniformly distributed random int64 values.
func randomKeys(n int) []int64 {
	keys := make([]int64, n)
	var buf [8]byte
	for i := range keys {
		if _, err := rand.Read(buf[:]); err != nil {
			panic(err) // crypto/rand failing is not a condition this experiment can meaningfully recover from
		}
		keys[i] = int64(binary.BigEndian.Uint64(buf[:]))
	}
	return keys
}

// uuid4LikeKeys returns n random keys (same distribution as randomKeys,
// kept separate for labeling purposes).
func uuid4LikeKeys(n int) []int64 {
	return randomKeys(n)
}

// uuid7LikeKeys returns n keys with monotonically increasing high bits
// (timestamp) and random low bits, providing good B+ tree locality.
func uuid7LikeKeys(n int) []int64 {
	keys := make([]int64, n)
	startMillis := time.Now().UnixMilli()
	var randBuf [4]byte
	for i := range keys {
		// Strictly increasing timestamps ensure high bits dominate key ordering.
		millis := startMillis + int64(i)
		if _, err := rand.Read(randBuf[:]); err != nil {
			panic(err)
		}
		lowBits := int64(binary.BigEndian.Uint32(randBuf[:])) & 0xFFFFFFF
		keys[i] = (millis << 28) | lowBits
	}
	return keys
}
