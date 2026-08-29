package btree

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	values := []int64{
		0,
		5,
		-5,
		1,
		-1,
		9223372036854775807,  // max int64
		-9223372036854775808, // min int64
	}

	for _, v := range values {
		encoded := EncodeInt64(v)
		decoded := DecodeInt64(encoded)
		if decoded != v {
			t.Errorf("round trip failed: encoded %d, decoded got %d", v, decoded)
		}
	}
}

func TestCompareKeysOrdering(t *testing.T) {
	tests := []struct {
		a, b     int64
		expected int
	}{
		{-5, 5, -1},    // negative vs positive: negative should sort first
		{0, -1, 1},     // zero vs negative: zero should sort after
		{3, 3, 0},      // equal values
		{-100, -1, -1}, // two negatives: more negative sorts first
		{5, -5, 1},     // reverse of the first case
		{0, 0, 0},      // zero vs itself
	}

	for _, tt := range tests {
		encodedA := EncodeInt64(tt.a)
		encodedB := EncodeInt64(tt.b)
		got := CompareKeys(encodedA, encodedB)
		if got != tt.expected {
			t.Errorf("CompareKeys(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}
