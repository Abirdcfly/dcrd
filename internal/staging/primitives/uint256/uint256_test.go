// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uint256

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// hexToUint256 converts the passed hex string into a Uint256 and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called with hard-coded values.
func hexToUint256(s string) *Uint256 {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b := hexToBytes(s)
	if len(b) > 32 {
		panic("hex in source file overflows mod 2^256: " + s)
	}
	return new(Uint256).SetByteSlice(b)
}

// TestUint256SetUint64 ensures that setting a scalar to various native integers
// works as expected.
func TestUint256SetUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string    // test description
		n    uint64    // test value
		want [4]uint64 // expected words
	}{{
		name: "five",
		n:    0x5,
		want: [4]uint64{0x5, 0, 0, 0},
	}, {
		name: "2^32 - 1",
		n:    0xffffffff,
		want: [4]uint64{0xffffffff, 0, 0, 0},
	}, {
		name: "2^32",
		n:    0x100000000,
		want: [4]uint64{0x100000000, 0, 0, 0},
	}, {
		name: "2^64 - 1",
		n:    0xffffffffffffffff,
		want: [4]uint64{0xffffffffffffffff, 0, 0, 0},
	}}

	for _, test := range tests {
		n := new(Uint256).SetUint64(test.n)
		if !reflect.DeepEqual(n.n, test.want) {
			t.Errorf("%s: wrong result -- got: %x want: %x", test.name, n.n,
				test.want)
			continue
		}
	}
}

// TestUint256SetBytes ensures that setting a uint256 to a 256-bit big-endian
// unsigned integer via both the slice and array methods works as expected for
// edge cases.  Random cases are tested via the various other tests.
func TestUint256SetBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string    // test description
		in   string    // hex encoded test value
		want [4]uint64 // expected words
	}{{
		name: "empty",
		in:   "",
		want: [4]uint64{0, 0, 0, 0},
	}, {
		name: "zero",
		in:   "00",
		want: [4]uint64{0, 0, 0, 0},
	}, {
		name: "one",
		in:   "0000000000000000000000000000000000000000000000000000000000000001",
		want: [4]uint64{1, 0, 0, 0},
	}, {
		name: "2^64-1 (no leading zeros)",
		in:   "ffffffffffffffff",
		want: [4]uint64{0xffffffffffffffff, 0, 0, 0},
	}, {
		name: "2^128-1 (with leading zeros)",
		in:   "00000000000000000000000000000000ffffffffffffffffffffffffffffffff",
		want: [4]uint64{0xffffffffffffffff, 0xffffffffffffffff, 0, 0},
	}, {
		name: "2^256 - 1",
		in:   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: [4]uint64{
			0xffffffffffffffff, 0xffffffffffffffff,
			0xffffffffffffffff, 0xffffffffffffffff,
		},
	}, {
		name: "2^8 - 1 (truncated >32 bytes)",
		in:   "0100000000000000000000000000000000000000000000000000000000000000ff",
		want: [4]uint64{0xff, 0, 0, 0},
	}, {
		name: "progression",
		in:   "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		want: [4]uint64{
			0x191a1b1c1d1e1f20, 0x1112131415161718,
			0x090a0b0c0d0e0f10, 0x0102030405060708,
		},
	}, {
		name: "alternating bits",
		in:   "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
		want: [4]uint64{
			0xa5a5a5a5a5a5a5a5, 0xa5a5a5a5a5a5a5a5,
			0xa5a5a5a5a5a5a5a5, 0xa5a5a5a5a5a5a5a5,
		},
	}, {
		name: "alternating bits 2",
		in:   "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		want: [4]uint64{
			0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a,
			0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a,
		},
	}}

	for _, test := range tests {
		inBytes := hexToBytes(test.in)

		// Ensure setting the bytes via the slice method works as expected.
		var n Uint256
		n.SetByteSlice(inBytes)
		if !reflect.DeepEqual(n.n, test.want) {
			t.Errorf("%s: unexpected result -- got: %x, want: %x", test.name,
				n.n, test.want)
			continue
		}

		// Ensure setting the bytes via the array method works as expected.
		var n2 Uint256
		var b32 [32]byte
		truncatedInBytes := inBytes
		if len(truncatedInBytes) > 32 {
			truncatedInBytes = truncatedInBytes[len(truncatedInBytes)-32:]
		}
		copy(b32[32-len(truncatedInBytes):], truncatedInBytes)
		n2.SetBytes(&b32)
		if !reflect.DeepEqual(n2.n, test.want) {
			t.Errorf("%s: unexpected result -- got: %x, want: %x", test.name,
				n2.n, test.want)
			continue
		}
	}
}

// TestUint256SetBytesLE ensures that setting a uint256 to a 256-bit
// little-endian unsigned integer via both the slice and array methods works as
// expected for edge cases.
func TestUint256SetBytesLE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string    // test description
		in   string    // hex encoded test value
		want [4]uint64 // expected words
	}{{
		name: "empty",
		in:   "",
		want: [4]uint64{0, 0, 0, 0},
	}, {
		name: "zero",
		in:   "00",
		want: [4]uint64{0, 0, 0, 0},
	}, {
		name: "one",
		in:   "01",
		want: [4]uint64{1, 0, 0, 0},
	}, {
		name: "2^64-1 (no trailing zeros)",
		in:   "ffffffffffffffff",
		want: [4]uint64{0xffffffffffffffff, 0, 0, 0},
	}, {
		name: "2^128-1 (with trailing zeros)",
		in:   "ffffffffffffffffffffffffffffffff00000000000000000000000000000000",
		want: [4]uint64{0xffffffffffffffff, 0xffffffffffffffff, 0, 0},
	}, {
		name: "2^256 - 1",
		in:   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: [4]uint64{
			0xffffffffffffffff, 0xffffffffffffffff,
			0xffffffffffffffff, 0xffffffffffffffff,
		},
	}, {
		name: "one (truncated >32 bytes)",
		in:   "0100000000000000000000000000000000000000000000000000000000000000ff",
		want: [4]uint64{1, 0, 0, 0},
	}, {
		name: "progression",
		in:   "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		want: [4]uint64{
			0x0807060504030201, 0x100f0e0d0c0b0a09,
			0x1817161514131211, 0x201f1e1d1c1b1a19,
		},
	}, {
		name: "alternating bits",
		in:   "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
		want: [4]uint64{
			0xa5a5a5a5a5a5a5a5, 0xa5a5a5a5a5a5a5a5,
			0xa5a5a5a5a5a5a5a5, 0xa5a5a5a5a5a5a5a5,
		},
	}, {
		name: "alternating bits 2",
		in:   "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		want: [4]uint64{
			0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a,
			0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a,
		},
	}}

	for _, test := range tests {
		inBytes := hexToBytes(test.in)

		// Ensure setting the bytes via the slice method works as expected.
		var n Uint256
		n.SetByteSliceLE(inBytes)
		if !reflect.DeepEqual(n.n, test.want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				n.n, test.want)
			continue
		}

		// Ensure setting the bytes via the array method works as expected.
		var n2 Uint256
		var b32 [32]byte
		truncatedInBytes := inBytes
		if len(truncatedInBytes) > 32 {
			truncatedInBytes = truncatedInBytes[:32]
		}
		copy(b32[:], truncatedInBytes)
		n2.SetBytesLE(&b32)
		if !reflect.DeepEqual(n2.n, test.want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				n2.n, test.want)
			continue
		}
	}
}

// TestUint256Bytes ensures that retrieving the bytes for a uint256 encoded as a
// 256-bit big-endian unsigned integer via the various methods works as expected
// for edge cases.  Random cases are tested via the various other tests.
func TestUint256Bytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		in   string // hex encoded test value
		want string // expected hex encoded bytes
	}{{
		name: "zero",
		in:   "0",
		want: "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "one",
		in:   "1",
		want: "0000000000000000000000000000000000000000000000000000000000000001",
	}, {
		name: "2^64 - 1",
		in:   "000000000000000000000000000000000000000000000000ffffffffffffffff",
		want: "000000000000000000000000000000000000000000000000ffffffffffffffff",
	}, {
		name: "2^128 - 1",
		in:   "00000000000000000000000000000000ffffffffffffffffffffffffffffffff",
		want: "00000000000000000000000000000000ffffffffffffffffffffffffffffffff",
	}, {
		name: "2^192 - 1",
		in:   "0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff",
		want: "0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "2^256 - 1",
		in:   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "alternating bits",
		in:   "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
		want: "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
	}, {
		name: "alternating bits 2",
		in:   "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		want: "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
	}}

	for _, test := range tests {
		n := hexToUint256(test.in)
		want := hexToBytes(test.want)

		// Ensure getting the bytes works as expected.
		gotBytes := n.Bytes()
		if !bytes.Equal(gotBytes[:], want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				gotBytes, want)
			continue
		}

		// Ensure getting the bytes directly into an array works as expected.
		var b32 [32]byte
		n.PutBytes(&b32)
		if !bytes.Equal(b32[:], want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				b32, want)
			continue
		}

		// Ensure getting the bytes directly into a slice works as expected.
		var buffer [64]byte
		n.PutBytesUnchecked(buffer[:])
		if !bytes.Equal(buffer[:32], want) {
			t.Errorf("%q: unexpected result, got: %x, want: %x", test.name,
				buffer[:32], want)
			continue
		}
	}
}

// TestUint256BytesLE ensures that retrieving the bytes for a uint256 encoded as
// a 256-bit little-endian unsigned integer via the various methods works as
// expected for edge cases.
func TestUint256BytesLE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		in   string // hex encoded test value
		want string // expected hex encoded bytes
	}{{
		name: "zero",
		in:   "0",
		want: "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "one",
		in:   "1",
		want: "0100000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "2^64 - 1",
		in:   "000000000000000000000000000000000000000000000000ffffffffffffffff",
		want: "ffffffffffffffff000000000000000000000000000000000000000000000000",
	}, {
		name: "2^128 - 1",
		in:   "00000000000000000000000000000000ffffffffffffffffffffffffffffffff",
		want: "ffffffffffffffffffffffffffffffff00000000000000000000000000000000",
	}, {
		name: "2^192 - 1",
		in:   "0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff",
		want: "ffffffffffffffffffffffffffffffffffffffffffffffff0000000000000000",
	}, {
		name: "2^256 - 1",
		in:   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "alternating bits",
		in:   "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
		want: "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
	}, {
		name: "alternating bits 2",
		in:   "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		want: "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
	}}

	for _, test := range tests {
		n := hexToUint256(test.in)
		want := hexToBytes(test.want)

		// Ensure getting the bytes works as expected.
		gotBytes := n.BytesLE()
		if !bytes.Equal(gotBytes[:], want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				gotBytes, want)
			continue
		}

		// Ensure getting the bytes directly into an array works as expected.
		var b32 [32]byte
		n.PutBytesLE(&b32)
		if !bytes.Equal(b32[:], want) {
			t.Errorf("%q: unexpected result -- got: %x, want: %x", test.name,
				b32, want)
			continue
		}

		// Ensure getting the bytes directly into a slice works as expected.
		var buffer [64]byte
		n.PutBytesUncheckedLE(buffer[:])
		if !bytes.Equal(buffer[:32], want) {
			t.Errorf("%q: unexpected result, got: %x, want: %x", test.name,
				buffer[:32], want)
			continue
		}
	}
}

// TestUint256Zero ensures that zeroing a uint256 works as expected.
func TestUint256Zero(t *testing.T) {
	t.Parallel()

	n := hexToUint256("a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5")
	n.Zero()
	for idx, word := range n.n {
		if word != 0 {
			t.Errorf("internal word at index #%d is not zero - got %d", idx,
				word)
		}
	}
}

// TestUint256IsZero ensures that checking if a uint256 is zero works as
// expected.
func TestUint256IsZero(t *testing.T) {
	t.Parallel()

	var n Uint256
	if !n.IsZero() {
		t.Fatalf("new uint256 is not zero - got %v (words %x)", n, n.n)
	}

	n.SetUint64(1)
	if n.IsZero() {
		t.Fatalf("claims zero for nonzero uint256 - got %v (words %x)", n, n.n)
	}

	n.SetUint64(0)
	if !n.IsZero() {
		t.Fatalf("claims nonzero for zero uint256 - got %v (words %x)", n, n.n)
	}

	n.SetUint64(1)
	n.Zero()
	if !n.IsZero() {
		t.Fatalf("claims zero for nonzero uint256 - got %v (words %x)", n, n.n)
	}
}

// TestUint256IsUint32 ensures that checking if a uint256 can be represented as
// a uint32 without loss of precision works as expected.
func TestUint256IsUint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		n    string // hex encoded value
		want bool   // expected result
	}{{
		name: "zero",
		n:    "0",
		want: true,
	}, {
		name: "one",
		n:    "1",
		want: true,
	}, {
		name: "two",
		n:    "2",
		want: true,
	}, {
		name: "2^32 - 1",
		n:    "ffffffff",
		want: true,
	}, {
		name: "2^32",
		n:    "100000000",
		want: false,
	}, {
		name: "2^64 - 2",
		n:    "fffffffffffffffe",
		want: false,
	}, {
		name: "2^128",
		n:    "100000000000000000000000000000000",
		want: false,
	}, {
		name: "2^128 - 1",
		n:    "ffffffffffffffffffffffffffffffff",
		want: false,
	}, {
		name: "2^256 - 1",
		n:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: false,
	}}

	for _, test := range tests {
		got := hexToUint256(test.n).IsUint32()
		if got != test.want {
			t.Errorf("%q: wrong result -- got: %v, want: %v", test.name, got,
				test.want)
			continue
		}
	}
}

// TestUint256Uint32 ensures that treating a uint256 as a uint32 produces the
// expected result.
func TestUint256Uint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		n    string // hex encoded value
		want uint32 // expected uint32
	}{{
		name: "zero",
		n:    "0",
		want: 0,
	}, {
		name: "one",
		n:    "1",
		want: 1,
	}, {
		name: "two",
		n:    "2",
		want: 2,
	}, {
		name: "2^32 - 1",
		n:    "ffffffff",
		want: 0xffffffff,
	}, {
		name: "2^32",
		n:    "100000000",
		want: 0,
	}, {
		name: "2^64 - 2",
		n:    "fffffffffffffffe",
		want: 0xfffffffe,
	}, {
		name: "2^128 - 1",
		n:    "ffffffffffffffffffffffffffffffff",
		want: 0xffffffff,
	}, {
		name: "2^128",
		n:    "100000000000000000000000000000000",
		want: 0,
	}, {
		name: "2^192 - 2^16 + 1",
		n:    "ffffffffffffffffffffffffffffffffffffffffffff0000",
		want: 0xffff0000,
	}, {
		name: "2^256 - 2^31",
		n:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffff7fffffff",
		want: 0x7fffffff,
	}}

	for _, test := range tests {
		got := hexToUint256(test.n).Uint32()
		if got != test.want {
			t.Errorf("%q: wrong result -- got: %x, want: %x", test.name, got,
				test.want)
			continue
		}
	}
}

// TestUint256IsUint64 ensures that checking if a uint256 can be represented as
// a uint64 without loss of precision works as expected.
func TestUint256IsUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		n    string // hex encoded value
		want bool   // expected result
	}{{
		name: "zero",
		n:    "0",
		want: true,
	}, {
		name: "one",
		n:    "1",
		want: true,
	}, {
		name: "two",
		n:    "2",
		want: true,
	}, {
		name: "2^32 - 1",
		n:    "ffffffff",
		want: true,
	}, {
		name: "2^32",
		n:    "100000000",
		want: true,
	}, {
		name: "2^64 - 2",
		n:    "fffffffffffffffe",
		want: true,
	}, {
		name: "2^64 - 1",
		n:    "ffffffffffffffff",
		want: true,
	}, {
		name: "2^64",
		n:    "10000000000000000",
		want: false,
	}, {
		name: "2^128",
		n:    "100000000000000000000000000000000",
		want: false,
	}, {
		name: "2^128 - 1",
		n:    "ffffffffffffffffffffffffffffffff",
		want: false,
	}, {
		name: "2^256 - 1",
		n:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		want: false,
	}}

	for _, test := range tests {
		got := hexToUint256(test.n).IsUint64()
		if got != test.want {
			t.Errorf("%q: wrong result -- got: %v, want: %v", test.name, got,
				test.want)
			continue
		}
	}
}

// TestUint256Uint64 ensures that treating a uint256 as a uint64 produces the
// expected result.
func TestUint256Uint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		n    string // hex encoded value
		want uint64 // expected uint64
	}{{
		name: "zero",
		n:    "0",
		want: 0,
	}, {
		name: "one",
		n:    "1",
		want: 1,
	}, {
		name: "two",
		n:    "2",
		want: 2,
	}, {
		name: "2^32 - 1",
		n:    "ffffffff",
		want: 0xffffffff,
	}, {
		name: "2^32",
		n:    "100000000",
		want: 0x100000000,
	}, {
		name: "2^64 - 2",
		n:    "fffffffffffffffe",
		want: 0xfffffffffffffffe,
	}, {
		name: "2^64 - 1",
		n:    "ffffffffffffffff",
		want: 0xffffffffffffffff,
	}, {
		name: "2^64",
		n:    "10000000000000000",
		want: 0,
	}, {
		name: "2^128 - 1",
		n:    "ffffffffffffffffffffffffffffffff",
		want: 0xffffffffffffffff,
	}, {
		name: "2^128",
		n:    "100000000000000000000000000000000",
		want: 0,
	}, {
		name: "2^192 - 2^16 + 1",
		n:    "ffffffffffffffffffffffffffffffffffffffffffff0000",
		want: 0xffffffffffff0000,
	}, {
		name: "2^256 - 2^63",
		n:    "ffffffffffffffffffffffffffffffffffffffffffffffff7fffffffffffffff",
		want: 0x7fffffffffffffff,
	}}

	for _, test := range tests {
		got := hexToUint256(test.n).Uint64()
		if got != test.want {
			t.Errorf("%q: wrong result -- got: %x, want: %x", test.name, got,
				test.want)
			continue
		}
	}
}
