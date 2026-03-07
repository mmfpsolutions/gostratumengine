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
	"math"
	"math/big"

	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
)

// RTT filter coefficients for each time window.
// These coefficients compute the Real Time Target based on
// time elapsed since recent blocks were found.
var rttFilterCoefficients = []float64{
	5.0372626864e-11, // 1-block window  (index 0, used after Nov 2025 upgrade)
	4.9192018423e-14, // 2-blocks window (index 1)
	4.8039080491e-17, // 5-blocks window (index 2)
	4.9192018423e-19, // 11-blocks window (index 3)
	4.6913164542e-20, // 17-blocks window (index 4)
}

// IsRTTDataValid checks if the RTT data from the node is usable.
// Returns false if all timestamps are identical, which happens when the node
// received all blocks at once during sync rather than naturally over time.
func IsRTTDataValid(template *noderpc.BlockTemplate) bool {
	if template == nil || template.RTT == nil {
		return false
	}
	if len(template.RTT.PrevHeaderTime) < 2 {
		return false
	}

	firstTs := template.RTT.PrevHeaderTime[0]
	for i := 1; i < len(template.RTT.PrevHeaderTime); i++ {
		if template.RTT.PrevHeaderTime[i] != firstTs {
			return true
		}
	}
	return false
}

// rttCompactToBig converts a compact target representation to a big.Int.
func rttCompactToBig(compact string) (*big.Int, error) {
	if len(compact) != 8 {
		return nil, fmt.Errorf("compact target must be 8 hex characters, got %d", len(compact))
	}
	data, err := hex.DecodeString(compact)
	if err != nil {
		return nil, fmt.Errorf("invalid hex in compact target: %w", err)
	}
	exponent := int(data[0])
	mantissa := new(big.Int).SetBytes(data[1:4])
	if mantissa.Sign() == 0 {
		return big.NewInt(0), nil
	}
	if exponent >= 3 {
		mantissa.Lsh(mantissa, uint((exponent-3)*8))
	} else {
		mantissa.Rsh(mantissa, uint((3-exponent)*8))
	}
	return mantissa, nil
}

// ComputeRTT computes the Real Time Target using the eCash formula.
// Uses the provided currentTime (unix seconds) for the calculation.
func ComputeRTT(template *noderpc.BlockTemplate, currentTime int64) (*big.Int, error) {
	if template == nil || template.RTT == nil {
		return nil, fmt.Errorf("no RTT data in template")
	}
	if len(template.RTT.PrevHeaderTime) == 0 {
		return nil, fmt.Errorf("empty prevheadertime array")
	}

	prevTarget, err := rttCompactToBig(template.RTT.PrevBits)
	if err != nil {
		return nil, fmt.Errorf("parsing prevbits '%s': %w", template.RTT.PrevBits, err)
	}

	nextTarget, err := rttCompactToBig(template.Bits)
	if err != nil {
		return nil, fmt.Errorf("parsing bits '%s': %w", template.Bits, err)
	}

	numWindows := len(template.RTT.PrevHeaderTime)

	// Before Nov 2025 upgrade: 4 windows, skip first coefficient (start at index 1)
	// After Nov 2025 upgrade: 5 windows, use all coefficients (start at index 0)
	filterIndex := 0
	if numWindows <= 4 {
		filterIndex = 1
	}

	prevTargetFloat := new(big.Float).SetInt(prevTarget)
	prevWindowTimestamp := int64(0)

	for i := 0; i < numWindows; i++ {
		if filterIndex >= len(rttFilterCoefficients) {
			break
		}

		timestamp := template.RTT.PrevHeaderTime[i]

		if timestamp == 0 {
			filterIndex++
			continue
		}
		if i > 0 && timestamp == prevWindowTimestamp {
			filterIndex++
			continue
		}
		prevWindowTimestamp = timestamp

		diffTime := currentTime - timestamp
		if diffTime < 1 {
			diffTime = 1
		}

		coeff := rttFilterCoefficients[filterIndex]
		filterIndex++

		diffTimePow5 := math.Pow(float64(diffTime), 5)
		result := new(big.Float).Mul(prevTargetFloat, big.NewFloat(coeff))
		result.Mul(result, big.NewFloat(diffTimePow5))

		target, _ := result.Int(nil)

		// RTT is never higher (less difficult) than the normal target
		if target.Cmp(nextTarget) < 0 {
			nextTarget = target
		}
	}

	return nextTarget, nil
}

// GetRTTTarget returns the RTT target, preferring the node's pre-computed nexttarget.
// Falls back to computing locally if nexttarget is not available.
func GetRTTTarget(template *noderpc.BlockTemplate, currentTime int64) (*big.Int, error) {
	if template == nil || template.RTT == nil {
		return nil, fmt.Errorf("no RTT data in template")
	}

	if !IsRTTDataValid(template) {
		return nil, fmt.Errorf("RTT data malformed - all timestamps identical")
	}

	// Prefer the node's pre-computed nexttarget
	if template.RTT.NextTarget != "" {
		target, err := rttCompactToBig(template.RTT.NextTarget)
		if err == nil && target.Sign() > 0 {
			return target, nil
		}
	}

	// Fallback: compute locally
	return ComputeRTT(template, currentTime)
}

// CheckRTTTarget validates if a block hash meets the RTT target.
// blockHashBE should be in big-endian byte order.
func CheckRTTTarget(blockHashBE []byte, template *noderpc.BlockTemplate, currentTime int64) (bool, error) {
	rttTarget, err := GetRTTTarget(template, currentTime)
	if err != nil {
		return false, err
	}

	hashInt := new(big.Int).SetBytes(blockHashBE)
	return hashInt.Cmp(rttTarget) <= 0, nil
}
