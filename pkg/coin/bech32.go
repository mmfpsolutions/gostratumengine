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
	"fmt"
	"strings"
)

// Bech32 encoding/decoding for SegWit addresses (BIP173).

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

var bech32CharsetRev [128]int8

func init() {
	for i := range bech32CharsetRev {
		bech32CharsetRev[i] = -1
	}
	for i, c := range bech32Charset {
		bech32CharsetRev[c] = int8(i)
	}
}

func bech32Polymod(values []int) int {
	gen := [5]int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := 1
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ v
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []int {
	ret := make([]int, 0, len(hrp)*2+1)
	for _, c := range hrp {
		ret = append(ret, int(c>>5))
	}
	ret = append(ret, 0)
	for _, c := range hrp {
		ret = append(ret, int(c&31))
	}
	return ret
}

func bech32VerifyChecksum(hrp string, data []int) bool {
	values := append(bech32HRPExpand(hrp), data...)
	return bech32Polymod(values) == 1
}

func bech32CreateChecksum(hrp string, data []int) []int {
	values := append(bech32HRPExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	polymod := bech32Polymod(values) ^ 1
	ret := make([]int, 6)
	for i := 0; i < 6; i++ {
		ret[i] = (polymod >> uint(5*(5-i))) & 31
	}
	return ret
}

// Bech32Decode decodes a Bech32 string, returning the HRP and data.
func Bech32Decode(bech string) (string, []int, error) {
	if len(bech) > 90 {
		return "", nil, fmt.Errorf("bech32 string too long")
	}

	lower := strings.ToLower(bech)
	upper := strings.ToUpper(bech)
	if bech != lower && bech != upper {
		return "", nil, fmt.Errorf("mixed case in bech32 string")
	}
	bech = lower

	pos := strings.LastIndex(bech, "1")
	if pos < 1 || pos+7 > len(bech) {
		return "", nil, fmt.Errorf("invalid bech32 separator position")
	}

	hrp := bech[:pos]
	dataStr := bech[pos+1:]

	data := make([]int, len(dataStr))
	for i, c := range dataStr {
		if c > 127 || bech32CharsetRev[c] == -1 {
			return "", nil, fmt.Errorf("invalid bech32 character: %c", c)
		}
		data[i] = int(bech32CharsetRev[c])
	}

	if !bech32VerifyChecksum(hrp, data) {
		return "", nil, fmt.Errorf("invalid bech32 checksum")
	}

	return hrp, data[:len(data)-6], nil
}

// Bech32Encode encodes data with the given HRP into a Bech32 string.
func Bech32Encode(hrp string, data []int) string {
	checksum := bech32CreateChecksum(hrp, data)
	combined := append(data, checksum...)
	var result strings.Builder
	result.WriteString(hrp)
	result.WriteByte('1')
	for _, d := range combined {
		result.WriteByte(bech32Charset[d])
	}
	return result.String()
}

// ConvertBits performs bit conversion between groupings.
func ConvertBits(data []int, fromBits, toBits uint, pad bool) ([]int, error) {
	acc := 0
	bits := uint(0)
	var ret []int
	maxv := (1 << toBits) - 1

	for _, value := range data {
		if value < 0 || value>>fromBits != 0 {
			return nil, fmt.Errorf("invalid data value: %d", value)
		}
		acc = acc<<fromBits | value
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, (acc>>bits)&maxv)
		}
	}

	if pad {
		if bits > 0 {
			ret = append(ret, (acc<<(toBits-bits))&maxv)
		}
	} else if bits >= fromBits || (acc<<(toBits-bits))&maxv != 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	return ret, nil
}

// DecodeBech32Address decodes a Bech32 SegWit address and returns the witness version
// and witness program bytes.
func DecodeBech32Address(address, expectedHRP string) (byte, []byte, error) {
	hrp, data, err := Bech32Decode(address)
	if err != nil {
		return 0, nil, err
	}
	if hrp != expectedHRP {
		return 0, nil, fmt.Errorf("unexpected HRP: got %s, want %s", hrp, expectedHRP)
	}
	if len(data) < 1 {
		return 0, nil, fmt.Errorf("empty data")
	}

	witnessVersion := byte(data[0])
	program, err := ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return 0, nil, fmt.Errorf("converting bits: %w", err)
	}

	programBytes := make([]byte, len(program))
	for i, v := range program {
		programBytes[i] = byte(v)
	}

	if witnessVersion == 0 && len(programBytes) != 20 && len(programBytes) != 32 {
		return 0, nil, fmt.Errorf("invalid witness program length for v0: %d", len(programBytes))
	}

	return witnessVersion, programBytes, nil
}
