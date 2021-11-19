// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uint256

import (
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
