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
	"encoding/hex"
)

// CoinbaseOutput represents a single output in the coinbase transaction.
type CoinbaseOutput struct {
	Value  int64
	Script []byte
}

// BuildCoinbaseParts constructs a coinbase transaction split into two halves (coinb1 and coinb2)
// with space for extranonce1+extranonce2 between them.
//
// CRITICAL: The coinbase sent to miners is in txid format — NO marker/flag/witness bytes.
// The miner SHA256d's the raw coinb1+en1+en2+coinb2 to get the coinbase hash for the merkle root.
// For SegWit coins, the witness commitment OUTPUT is included in the outputs (it's part of the txid),
// but witness marker/flag/witness stack are only added during block submission (in BuildBlock).
func BuildCoinbaseParts(
	height int64,
	coinbaseText string,
	extraNonce1Size int,
	extraNonce2Size int,
	outputs []CoinbaseOutput,
	segwit bool,
) (coinb1Hex, coinb2Hex string) {
	// --- Build scriptSig ---
	heightBytes := SerializeHeight(height)
	textBytes := []byte(coinbaseText)

	// scriptSig = height + coinbase_text + [extranonce placeholder]
	totalExtraNonceSize := extraNonce1Size + extraNonce2Size
	scriptSigLen := len(heightBytes) + len(textBytes) + totalExtraNonceSize

	// --- coinb1: everything up to (but not including) the extranonce ---
	var coinb1 []byte

	// version (4) + vin_count (1) — NO marker/flag even for SegWit (txid format)
	coinb1 = append(coinb1, PutUint32LE(1)...) // version 1
	coinb1 = append(coinb1, 0x01)              // vin count = 1

	// prev_txid (32 bytes of zeros for coinbase)
	coinb1 = append(coinb1, make([]byte, 32)...)
	// prev_vout (0xFFFFFFFF for coinbase)
	coinb1 = append(coinb1, 0xFF, 0xFF, 0xFF, 0xFF)
	// scriptSig length
	coinb1 = append(coinb1, SerializeVarInt(uint64(scriptSigLen))...)
	// scriptSig prefix: height + coinbase text
	coinb1 = append(coinb1, heightBytes...)
	coinb1 = append(coinb1, textBytes...)

	// --- coinb2: everything after the extranonce ---
	var coinb2 []byte

	// sequence (0xFFFFFFFF)
	coinb2 = append(coinb2, 0xFF, 0xFF, 0xFF, 0xFF)

	// outputs (witness commitment output is included here for SegWit — it's part of the txid)
	coinb2 = append(coinb2, SerializeVarInt(uint64(len(outputs)))...)
	for _, out := range outputs {
		coinb2 = append(coinb2, PutUint64LE(uint64(out.Value))...)
		coinb2 = append(coinb2, SerializeVarInt(uint64(len(out.Script)))...)
		coinb2 = append(coinb2, out.Script...)
	}

	// NO witness data here — added in BuildBlock during block submission

	// locktime (0x00000000)
	coinb2 = append(coinb2, 0x00, 0x00, 0x00, 0x00)

	coinb1Hex = hex.EncodeToString(coinb1)
	coinb2Hex = hex.EncodeToString(coinb2)

	return coinb1Hex, coinb2Hex
}

// AssembleCoinbase combines coinb1 + extranonce1 + extranonce2 + coinb2 into a full
// coinbase transaction (as raw bytes). The result is in txid format (no witness data).
func AssembleCoinbase(coinb1Hex, extraNonce1, extraNonce2, coinb2Hex string) ([]byte, error) {
	fullHex := coinb1Hex + extraNonce1 + extraNonce2 + coinb2Hex
	return hex.DecodeString(fullHex)
}

// AddWitnessData adds SegWit marker/flag and witness data to a coinbase transaction
// for block submission. The coinbase already has the witness commitment OUTPUT
// (from BuildCoinbaseParts). This adds: marker(1) + flag(1) after version,
// and witness stack (34 bytes) before locktime.
//
// Input:  version(4) + inputs + outputs(including witness commitment) + locktime(4)
// Output: version(4) + marker(1) + flag(1) + inputs + outputs + witness(34) + locktime(4)
func AddWitnessData(cb []byte) []byte {
	if len(cb) < 10 {
		return cb
	}

	result := make([]byte, 0, len(cb)+36)

	// Version (4 bytes)
	result = append(result, cb[0:4]...)

	// Add marker and flag
	result = append(result, 0x00, 0x01)

	// Copy everything between version and locktime (inputs + outputs)
	result = append(result, cb[4:len(cb)-4]...)

	// Add witness stack: 1 element, 32-byte witness reserved value (all zeros)
	result = append(result, 0x01)                // 1 stack element
	result = append(result, 0x20)                // 32 bytes
	result = append(result, make([]byte, 32)...) // 32 zero bytes

	// Locktime (last 4 bytes)
	result = append(result, cb[len(cb)-4:]...)

	return result
}

// readVarInt reads a Bitcoin variable-length integer and returns the value and bytes consumed.
func readVarInt(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}
	first := data[0]
	switch {
	case first < 0xFD:
		return uint64(first), 1
	case first == 0xFD:
		if len(data) < 3 {
			return 0, 1
		}
		return uint64(data[1]) | uint64(data[2])<<8, 3
	case first == 0xFE:
		if len(data) < 5 {
			return 0, 1
		}
		return uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24, 5
	default:
		if len(data) < 9 {
			return 0, 1
		}
		return uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24 |
			uint64(data[5])<<32 | uint64(data[6])<<40 | uint64(data[7])<<48 | uint64(data[8])<<56, 9
	}
}

// MerkleRoot computes the merkle root given a list of transaction hashes (as byte slices).
// The first hash should be the coinbase txid.
func MerkleRoot(hashes [][]byte) []byte {
	if len(hashes) == 0 {
		return make([]byte, 32)
	}
	if len(hashes) == 1 {
		result := make([]byte, 32)
		copy(result, hashes[0])
		return result
	}

	for len(hashes) > 1 {
		if len(hashes)%2 != 0 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}
		var next [][]byte
		for i := 0; i < len(hashes); i += 2 {
			combined := make([]byte, 64)
			copy(combined[:32], hashes[i])
			copy(combined[32:], hashes[i+1])
			hash := DoubleSHA256(combined)
			next = append(next, hash)
		}
		hashes = next
	}
	return hashes[0]
}

// ComputeMerkleRootFromBranches computes the merkle root given a coinbase hash
// and a set of merkle branches (as returned in stratum mining.notify).
func ComputeMerkleRootFromBranches(coinbaseHash []byte, branches [][]byte) []byte {
	result := make([]byte, 32)
	copy(result, coinbaseHash)

	for _, branch := range branches {
		combined := make([]byte, 64)
		copy(combined[:32], result)
		copy(combined[32:], branch)
		result = DoubleSHA256(combined)
	}

	return result
}
