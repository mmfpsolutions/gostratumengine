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
	"math"
	"math/big"
	"testing"
)

func TestCompactToBig(t *testing.T) {
	tests := []struct {
		name    string
		compact uint32
		want    string // hex representation
	}{
		{
			name:    "genesis block target",
			compact: 0x1d00ffff,
			want:    "FFFF0000000000000000000000000000000000000000000000000000",
		},
		{
			name:    "typical mainnet target",
			compact: 0x17034267,
			want:    "342670000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompactToBig(tt.compact)
			want := new(big.Int)
			want.SetString(tt.want, 16)
			if got.Cmp(want) != 0 {
				t.Errorf("CompactToBig(0x%08x) = %s, want %s", tt.compact, got.Text(16), want.Text(16))
			}
		})
	}
}

func TestCompactToDifficulty(t *testing.T) {
	// Genesis block should have difficulty ~1
	diff := CompactToDifficulty(0x1d00ffff)
	if math.Abs(diff-1.0) > 0.001 {
		t.Errorf("genesis block difficulty = %f, want ~1.0", diff)
	}
}

func TestDifficultyToTarget(t *testing.T) {
	// Difficulty 1 should give us MaxTarget
	target := DifficultyToTarget(1.0)
	if target.Cmp(MaxTarget) != 0 {
		t.Errorf("DifficultyToTarget(1) != MaxTarget")
	}

	// Difficulty 2 should give half of MaxTarget
	target2 := DifficultyToTarget(2.0)
	halfMax := new(big.Int).Div(MaxTarget, big.NewInt(2))
	if target2.Cmp(halfMax) != 0 {
		t.Errorf("DifficultyToTarget(2) = %s, want %s", target2.Text(16), halfMax.Text(16))
	}
}

func TestBitsToHex(t *testing.T) {
	compact, err := BitsToHex("1d00ffff")
	if err != nil {
		t.Fatalf("BitsToHex error: %v", err)
	}
	if compact != 0x1d00ffff {
		t.Errorf("BitsToHex(\"1d00ffff\") = 0x%08x, want 0x1d00ffff", compact)
	}
}

func TestHashMeetsDifficulty(t *testing.T) {
	// A hash of all zeros meets any difficulty
	zeroHash := make([]byte, 32)
	if !HashMeetsDifficulty(zeroHash, 1.0) {
		t.Error("zero hash should meet difficulty 1")
	}

	// A hash of all 0xFF should not meet any meaningful difficulty
	maxHash := make([]byte, 32)
	for i := range maxHash {
		maxHash[i] = 0xFF
	}
	if HashMeetsDifficulty(maxHash, 1.0) {
		t.Error("max hash should not meet difficulty 1")
	}
}
