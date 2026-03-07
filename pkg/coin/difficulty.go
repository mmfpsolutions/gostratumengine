/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package coin

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

// MaxTarget is the maximum target for SHA256d (difficulty 1).
// This is 0x00000000FFFF0000000000000000000000000000000000000000000000000000
var MaxTarget *big.Int

func init() {
	MaxTarget = new(big.Int)
	MaxTarget.SetString("00000000FFFF0000000000000000000000000000000000000000000000000000", 16)
}

// CompactToBig converts a compact target representation (nBits) to a big.Int.
func CompactToBig(compact uint32) *big.Int {
	exponent := compact >> 24
	mantissa := compact & 0x007FFFFF

	var target big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		target.SetInt64(int64(mantissa))
	} else {
		target.SetInt64(int64(mantissa))
		target.Lsh(&target, uint(8*(exponent-3)))
	}

	if compact&0x00800000 != 0 {
		target.Neg(&target)
	}

	return &target
}

// CompactToDifficulty converts a compact target (nBits) to pool difficulty.
func CompactToDifficulty(compact uint32) float64 {
	target := CompactToBig(compact)
	if target.Sign() == 0 {
		return 0
	}
	diff := new(big.Float).SetInt(MaxTarget)
	diff.Quo(diff, new(big.Float).SetInt(target))
	result, _ := diff.Float64()
	return result
}

// DifficultyToTarget converts a pool difficulty to a 256-bit target as big.Int.
func DifficultyToTarget(difficulty float64) *big.Int {
	if difficulty <= 0 {
		return new(big.Int).Set(MaxTarget)
	}
	// target = MaxTarget / difficulty
	diffBig := new(big.Float).SetFloat64(difficulty)
	maxFloat := new(big.Float).SetInt(MaxTarget)
	result := new(big.Float).Quo(maxFloat, diffBig)

	target, _ := result.Int(nil)
	return target
}

// BitsToHex converts a bits string (e.g., "1a0377ae") to a compact uint32.
func BitsToHex(bits string) (uint32, error) {
	b, err := hex.DecodeString(bits)
	if err != nil || len(b) != 4 {
		return 0, fmt.Errorf("invalid bits string: %s", bits)
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

// TargetToHex converts a big.Int target to a 64-character hex string (32 bytes, big-endian).
func TargetToHex(target *big.Int) string {
	b := target.Bytes()
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return hex.EncodeToString(padded)
}

// HashMeetsTarget checks if a hash (as big-endian bytes) meets the given target.
func HashMeetsTarget(hash []byte, target *big.Int) bool {
	hashInt := new(big.Int).SetBytes(hash)
	return hashInt.Cmp(target) <= 0
}

// HashMeetsDifficulty checks if a hash (as big-endian bytes) meets the given pool difficulty.
func HashMeetsDifficulty(hash []byte, difficulty float64) bool {
	target := DifficultyToTarget(difficulty)
	return HashMeetsTarget(hash, target)
}
