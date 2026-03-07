/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package coinbase

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestP2PKHScript(t *testing.T) {
	// Standard P2PKH output script should be 25 bytes
	pubKeyHash := make([]byte, 20)
	script := P2PKHScript(pubKeyHash)

	if len(script) != 25 {
		t.Errorf("P2PKH script length = %d, want 25", len(script))
	}
	if script[0] != OpDup || script[1] != OpHash160 || script[2] != 0x14 {
		t.Error("P2PKH script prefix incorrect")
	}
	if script[23] != OpEqualVerify || script[24] != OpCheckSig {
		t.Error("P2PKH script suffix incorrect")
	}
}

func TestP2WPKHScript(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	script := P2WPKHScript(pubKeyHash)

	if len(script) != 22 {
		t.Errorf("P2WPKH script length = %d, want 22", len(script))
	}
	if script[0] != Op0 || script[1] != 0x14 {
		t.Error("P2WPKH script prefix incorrect")
	}
}

func TestSerializeHeight(t *testing.T) {
	tests := []struct {
		height int64
		want   string // hex
	}{
		{1, "0101"},
		{100, "0164"},
		{256, "020001"},
		{500000, "0320a107"},
	}

	for _, tt := range tests {
		got := hex.EncodeToString(SerializeHeight(tt.height))
		if got != tt.want {
			t.Errorf("SerializeHeight(%d) = %s, want %s", tt.height, got, tt.want)
		}
	}
}

func TestSerializeVarInt(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "00"},
		{1, "01"},
		{252, "fc"},
		{253, "fdfd00"},
		{65535, "fdffff"},
	}

	for _, tt := range tests {
		got := hex.EncodeToString(SerializeVarInt(tt.n))
		if got != tt.want {
			t.Errorf("SerializeVarInt(%d) = %s, want %s", tt.n, got, tt.want)
		}
	}
}

func TestDoubleSHA256(t *testing.T) {
	// Known test vector: SHA256d of empty bytes
	result := DoubleSHA256([]byte{})
	expected, _ := hex.DecodeString("5df6e0e2761359d30a8275058e299fcc0381534545f55cf43e41983f5d4c9456")
	if !bytes.Equal(result, expected) {
		t.Errorf("DoubleSHA256(empty) = %x, want %x", result, expected)
	}
}

func TestReverseBytes(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	expected := []byte{0x04, 0x03, 0x02, 0x01}
	result := ReverseBytes(input)
	if !bytes.Equal(result, expected) {
		t.Errorf("ReverseBytes = %x, want %x", result, expected)
	}
	// Original should be unchanged
	if !bytes.Equal(input, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Error("ReverseBytes modified original slice")
	}
}

func TestMerkleRoot(t *testing.T) {
	// Single hash should return itself
	hash := DoubleSHA256([]byte("test"))
	root := MerkleRoot([][]byte{hash})
	if !bytes.Equal(root, hash) {
		t.Error("single hash merkle root should equal the hash itself")
	}

	// Two hashes
	hash1 := DoubleSHA256([]byte("a"))
	hash2 := DoubleSHA256([]byte("b"))
	root2 := MerkleRoot([][]byte{hash1, hash2})
	if len(root2) != 32 {
		t.Errorf("merkle root length = %d, want 32", len(root2))
	}

	// Verify it's a deterministic function
	root2b := MerkleRoot([][]byte{hash1, hash2})
	if !bytes.Equal(root2, root2b) {
		t.Error("merkle root is not deterministic")
	}
}

func TestComputeMerkleRootFromBranches(t *testing.T) {
	// With no branches, the coinbase hash IS the merkle root
	cbHash := DoubleSHA256([]byte("coinbase"))
	root := ComputeMerkleRootFromBranches(cbHash, nil)
	if !bytes.Equal(root, cbHash) {
		t.Error("no branches should return coinbase hash")
	}
}
