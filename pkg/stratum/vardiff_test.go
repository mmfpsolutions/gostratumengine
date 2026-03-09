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
