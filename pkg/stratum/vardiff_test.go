/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package stratum

import (
	"testing"
	"time"
)

func TestVarDiffNoRetargetBeforeMinShares(t *testing.T) {
	vd := NewVarDiff(VarDiffConfig{
		MinDiff:      1,
		MaxDiff:      65536,
		TargetTime:   15,
		RetargetTime: 0.1, // Very short retarget time for testing
		VariancePct:  30,
	}, 1024)

	// Less than 4 shares should never retarget
	for i := 0; i < 3; i++ {
		if result := vd.RecordShare(); result.Adjusted {
			t.Errorf("retarget happened with only %d shares", i+1)
		}
	}
}

func TestVarDiffClampToMax(t *testing.T) {
	vd := NewVarDiff(VarDiffConfig{
		MinDiff:      1,
		MaxDiff:      2048,
		TargetTime:   15,
		RetargetTime: 0,
		VariancePct:  1,
	}, 1024)

	// Simulate very fast share submissions (should increase difficulty)
	now := time.Now()
	vd.shareTimes = []time.Time{
		now.Add(-1 * time.Millisecond),
		now.Add(-2 * time.Millisecond),
		now.Add(-3 * time.Millisecond),
		now.Add(-4 * time.Millisecond),
	}
	vd.lastRetarget = now.Add(-1 * time.Hour)

	result := vd.RecordShare()
	if result.ClampedDiff > 2048 {
		t.Errorf("difficulty %f exceeds max 2048", result.ClampedDiff)
	}
}

func TestVarDiffClampToMin(t *testing.T) {
	vd := NewVarDiff(VarDiffConfig{
		MinDiff:      512,
		MaxDiff:      65536,
		TargetTime:   15,
		RetargetTime: 0,
		VariancePct:  1,
	}, 1024)

	// Simulate very slow share submissions (should decrease difficulty)
	now := time.Now()
	vd.shareTimes = []time.Time{
		now.Add(-500 * time.Second),
		now.Add(-400 * time.Second),
		now.Add(-300 * time.Second),
		now.Add(-200 * time.Second),
	}
	vd.lastRetarget = now.Add(-1 * time.Hour)

	result := vd.RecordShare()
	if result.Adjusted && result.ClampedDiff < 512 {
		t.Errorf("difficulty %f below min 512", result.ClampedDiff)
	}
}

func TestVarDiffCurrentDiff(t *testing.T) {
	vd := NewVarDiff(VarDiffConfig{
		MinDiff:    1,
		MaxDiff:    65536,
		TargetTime: 15,
	}, 2048)

	if vd.CurrentDiff() != 2048 {
		t.Errorf("initial diff = %f, want 2048", vd.CurrentDiff())
	}
}

func TestVarDiffFloatMode(t *testing.T) {
	vd := NewVarDiff(VarDiffConfig{
		MinDiff:        0.01,
		MaxDiff:        100,
		TargetTime:     15,
		RetargetTime:   0,
		VariancePct:    1,
		FloatDiff:      true,
		FloatPrecision: 2,
	}, 1.0)

	// Simulate slow shares — should reduce below 1.0
	now := time.Now()
	vd.shareTimes = []time.Time{
		now.Add(-200 * time.Second),
		now.Add(-150 * time.Second),
		now.Add(-100 * time.Second),
		now.Add(-50 * time.Second),
	}
	vd.lastRetarget = now.Add(-1 * time.Hour)

	result := vd.RecordShare()
	if result.Adjusted && result.ClampedDiff < 0.01 {
		t.Errorf("float diff %f below min 0.01", result.ClampedDiff)
	}
}

func TestVarDiffFloatDiffBelowOne(t *testing.T) {
	// Test that floatDiffBelowOne rounds to integer for diff >= 1
	// but preserves float precision for sub-1 values
	t.Run("above_one_rounds_to_integer", func(t *testing.T) {
		vd := NewVarDiff(VarDiffConfig{
			MinDiff:           1,
			MaxDiff:           200000,
			TargetTime:        15,
			RetargetTime:      0,
			VariancePct:       1,
			FloatDiff:         true,
			FloatDiffBelowOne: true,
			FloatPrecision:    4,
		}, 50000)

		// Simulate fast shares — should increase difficulty
		now := time.Now()
		vd.shareTimes = []time.Time{
			now.Add(-8 * time.Second),
			now.Add(-6 * time.Second),
			now.Add(-4 * time.Second),
			now.Add(-2 * time.Second),
		}
		vd.lastRetarget = now.Add(-1 * time.Hour)

		result := vd.RecordShare()
		if result.Adjusted {
			// Difficulty should be a whole number (no decimals) when >= 1
			if result.ClampedDiff != float64(int(result.ClampedDiff)) {
				t.Errorf("floatDiffBelowOne: diff >= 1 should be integer, got %f", result.ClampedDiff)
			}
		}
	})

	t.Run("below_one_preserves_float", func(t *testing.T) {
		vd := NewVarDiff(VarDiffConfig{
			MinDiff:           0.001,
			MaxDiff:           100,
			TargetTime:        15,
			RetargetTime:      0,
			VariancePct:       1,
			FloatDiff:         true,
			FloatDiffBelowOne: true,
			FloatPrecision:    4,
		}, 0.5)

		// Simulate slow shares — should decrease difficulty below 1
		now := time.Now()
		vd.shareTimes = []time.Time{
			now.Add(-200 * time.Second),
			now.Add(-150 * time.Second),
			now.Add(-100 * time.Second),
			now.Add(-50 * time.Second),
		}
		vd.lastRetarget = now.Add(-1 * time.Hour)

		result := vd.RecordShare()
		if result.Adjusted {
			if result.ClampedDiff >= 1.0 {
				t.Errorf("expected sub-1 difficulty, got %f", result.ClampedDiff)
			}
			// Should not be rounded to integer
			if result.ClampedDiff == float64(int(result.ClampedDiff)) && result.ClampedDiff != 0 {
				t.Errorf("sub-1 float diff should have decimal precision, got %f", result.ClampedDiff)
			}
		}
	})

	t.Run("disabled_uses_float_everywhere", func(t *testing.T) {
		vd := NewVarDiff(VarDiffConfig{
			MinDiff:           1,
			MaxDiff:           200000,
			TargetTime:        15,
			RetargetTime:      0,
			VariancePct:       1,
			FloatDiff:         true,
			FloatDiffBelowOne: false,
			FloatPrecision:    4,
		}, 50000)

		// roundDifficulty should apply float precision even above 1
		rounded := vd.roundDifficulty(103297.1234)
		if rounded != 103297.1234 {
			t.Errorf("floatDiffBelowOne=false: expected 103297.1234, got %f", rounded)
		}
	})

	t.Run("enabled_integer_above_one", func(t *testing.T) {
		vd := NewVarDiff(VarDiffConfig{
			MinDiff:           1,
			MaxDiff:           200000,
			TargetTime:        15,
			FloatDiff:         true,
			FloatDiffBelowOne: true,
			FloatPrecision:    4,
		}, 50000)

		// roundDifficulty should return integer for values >= 1
		rounded := vd.roundDifficulty(103297.5678)
		if rounded != 103298 {
			t.Errorf("floatDiffBelowOne=true: expected 103298, got %f", rounded)
		}

		// But preserve float for sub-1
		rounded = vd.roundDifficulty(0.00384)
		if rounded != 0.0038 {
			t.Errorf("floatDiffBelowOne=true sub-1: expected 0.0038, got %f", rounded)
		}
	})
}
