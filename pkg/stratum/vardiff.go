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
	"fmt"
	"math"
	"time"
)

// VarDiffResult holds the outcome of a retarget check for diagnostic logging.
type VarDiffResult struct {
	Adjusted    bool    // whether difficulty was changed
	Reason      string  // "adjusted", "within_variance", "min_shares", "retarget_wait"
	Shares      int     // number of shares in window
	AvgTime     float64 // average seconds between shares
	TargetTime  float64 // target seconds between shares
	AcceptLow   float64 // lower bound of acceptable range
	AcceptHigh  float64 // upper bound of acceptable range
	CurrentDiff float64 // difficulty before adjustment
	CalcDiff    float64 // raw calculated difficulty (before clamping)
	ClampedDiff float64 // final difficulty (after clamping/rounding)
	ChangePct   float64 // percentage change
}

// DiagString returns a GSS-style diagnostic string for logging.
func (r VarDiffResult) DiagString() string {
	if r.Reason == "min_shares" || r.Reason == "retarget_wait" {
		return ""
	}
	return fmt.Sprintf("shares=%d avgTime=%.2fs target=%.0fs acceptable=[%.1f-%.1f] actualDiff=%.4f calcDiff=%.4f clampedDiff=%.4f change=%.1f%%",
		r.Shares, r.AvgTime, r.TargetTime, r.AcceptLow, r.AcceptHigh,
		r.CurrentDiff, r.CalcDiff, r.ClampedDiff, r.ChangePct*100)
}

// VarDiff manages variable difficulty for a single miner session.
type VarDiff struct {
	minDiff        float64
	maxDiff        float64
	targetTime     float64 // desired seconds between shares
	retargetTime   float64 // seconds between retarget checks
	variancePct    float64 // acceptable variance as fraction (e.g., 0.30)
	floatDiff      bool    // allow float difficulty values
	floatPrecision int     // decimal places for float diff

	currentDiff  float64
	shareTimes   []time.Time // timestamps of last N shares
	lastRetarget time.Time
	maxShares    int // max share timestamps to track
}

// VarDiffConfig holds configuration for variable difficulty.
type VarDiffConfig struct {
	MinDiff        float64
	MaxDiff        float64
	TargetTime     float64
	RetargetTime   float64
	VariancePct    float64
	FloatDiff      bool
	FloatPrecision int
}

// NewVarDiff creates a new VarDiff tracker with the given configuration.
func NewVarDiff(cfg VarDiffConfig, initialDiff float64) *VarDiff {
	return &VarDiff{
		minDiff:        cfg.MinDiff,
		maxDiff:        cfg.MaxDiff,
		targetTime:     cfg.TargetTime,
		retargetTime:   cfg.RetargetTime,
		variancePct:    cfg.VariancePct / 100.0, // convert from percentage
		floatDiff:      cfg.FloatDiff,
		floatPrecision: cfg.FloatPrecision,
		currentDiff:    initialDiff,
		shareTimes:     make([]time.Time, 0, 10),
		lastRetarget:   time.Now(),
		maxShares:      10,
	}
}

// CurrentDiff returns the current difficulty.
func (v *VarDiff) CurrentDiff() float64 {
	return v.currentDiff
}

// ResetWindow clears the share timestamp window and retarget timer.
// Called after a pending difficulty change is delivered so the new difficulty
// starts with a clean measurement window.
func (v *VarDiff) ResetWindow(newDiff float64) {
	v.currentDiff = newDiff
	v.shareTimes = v.shareTimes[:0]
	v.lastRetarget = time.Now()
}

// RecordShare records a share submission timestamp and returns a VarDiffResult
// describing the retarget decision. Result.Adjusted indicates if difficulty changed.
func (v *VarDiff) RecordShare() VarDiffResult {
	now := time.Now()

	v.shareTimes = append(v.shareTimes, now)
	if len(v.shareTimes) > v.maxShares {
		v.shareTimes = v.shareTimes[1:]
	}

	// Need at least 4 shares and retargetTime elapsed
	if len(v.shareTimes) < 4 {
		return VarDiffResult{Reason: "min_shares"}
	}

	elapsed := now.Sub(v.lastRetarget).Seconds()
	if elapsed < v.retargetTime {
		return VarDiffResult{Reason: "retarget_wait"}
	}

	// Calculate average time between shares
	avgTime := v.averageShareTime()
	if avgTime <= 0 {
		return VarDiffResult{Reason: "retarget_wait"}
	}

	acceptLow := v.targetTime * (1 - v.variancePct)
	acceptHigh := v.targetTime * (1 + v.variancePct)

	// Check if within acceptable variance
	variance := math.Abs(avgTime-v.targetTime) / v.targetTime
	if variance <= v.variancePct {
		v.lastRetarget = now
		return VarDiffResult{
			Reason:      "within_variance",
			Shares:      len(v.shareTimes),
			AvgTime:     avgTime,
			TargetTime:  v.targetTime,
			AcceptLow:   acceptLow,
			AcceptHigh:  acceptHigh,
			CurrentDiff: v.currentDiff,
			CalcDiff:    v.currentDiff,
			ClampedDiff: v.currentDiff,
		}
	}

	// Calculate adjustment ratio
	ratio := v.targetTime / avgTime

	// Clamp ratio to prevent extreme changes
	if ratio > 4.0 {
		ratio = 4.0
	}
	if ratio < 0.25 {
		ratio = 0.25
	}

	calcDiff := v.currentDiff * ratio
	newDiff := calcDiff

	// Apply min/max bounds
	if newDiff < v.minDiff {
		newDiff = v.minDiff
	}
	if newDiff > v.maxDiff {
		newDiff = v.maxDiff
	}

	// Round based on float mode
	if !v.floatDiff {
		newDiff = math.Round(newDiff)
		if newDiff < 1 {
			newDiff = 1
		}
	} else {
		// Round to configured precision
		p := math.Pow(10, float64(v.floatPrecision))
		newDiff = math.Round(newDiff*p) / p
	}

	// Skip if change is less than 1%
	change := math.Abs(newDiff-v.currentDiff) / v.currentDiff
	if change < 0.01 {
		v.lastRetarget = now
		return VarDiffResult{
			Reason:      "within_variance",
			Shares:      len(v.shareTimes),
			AvgTime:     avgTime,
			TargetTime:  v.targetTime,
			AcceptLow:   acceptLow,
			AcceptHigh:  acceptHigh,
			CurrentDiff: v.currentDiff,
			CalcDiff:    calcDiff,
			ClampedDiff: newDiff,
			ChangePct:   change,
		}
	}

	oldDiff := v.currentDiff
	v.currentDiff = newDiff
	v.lastRetarget = now
	v.shareTimes = v.shareTimes[:0] // reset share window

	return VarDiffResult{
		Adjusted:    true,
		Reason:      "adjusted",
		Shares:      0, // reset, so report what we had
		AvgTime:     avgTime,
		TargetTime:  v.targetTime,
		AcceptLow:   acceptLow,
		AcceptHigh:  acceptHigh,
		CurrentDiff: oldDiff,
		CalcDiff:    calcDiff,
		ClampedDiff: newDiff,
		ChangePct:   change,
	}
}

func (v *VarDiff) averageShareTime() float64 {
	if len(v.shareTimes) < 2 {
		return 0
	}

	totalTime := v.shareTimes[len(v.shareTimes)-1].Sub(v.shareTimes[0]).Seconds()
	intervals := float64(len(v.shareTimes) - 1)

	return totalTime / intervals
}
