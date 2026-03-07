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
	"math"
	"time"
)

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

// RecordShare records a share submission timestamp and returns a new difficulty
// if a retarget is needed, or 0 if no change.
func (v *VarDiff) RecordShare() float64 {
	now := time.Now()

	v.shareTimes = append(v.shareTimes, now)
	if len(v.shareTimes) > v.maxShares {
		v.shareTimes = v.shareTimes[1:]
	}

	// Need at least 4 shares and retargetTime elapsed
	if len(v.shareTimes) < 4 {
		return 0
	}

	elapsed := now.Sub(v.lastRetarget).Seconds()
	if elapsed < v.retargetTime {
		return 0
	}

	// Calculate average time between shares
	avgTime := v.averageShareTime()
	if avgTime <= 0 {
		return 0
	}

	// Check if within acceptable variance
	variance := math.Abs(avgTime-v.targetTime) / v.targetTime
	if variance <= v.variancePct {
		v.lastRetarget = now
		return 0
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

	newDiff := v.currentDiff * ratio

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
		return 0
	}

	v.currentDiff = newDiff
	v.lastRetarget = now
	v.shareTimes = v.shareTimes[:0] // reset share window

	return newDiff
}

func (v *VarDiff) averageShareTime() float64 {
	if len(v.shareTimes) < 2 {
		return 0
	}

	totalTime := v.shareTimes[len(v.shareTimes)-1].Sub(v.shareTimes[0]).Seconds()
	intervals := float64(len(v.shareTimes) - 1)

	return totalTime / intervals
}
