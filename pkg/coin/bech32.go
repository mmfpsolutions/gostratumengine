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

	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
)

// Bech32 (BIP173) and Bech32m (BIP350) encoding/decoding for SegWit addresses.

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// Checksum constants. The two encodings differ only in the value the
// checksum polymod must equal: witness version 0 addresses use Bech32,
// versions 1 and up (Taproot is version 1) use Bech32m.
const (
	bech32Const  = 1
	bech32mConst = 0x2bc830a3
)

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
	return bech32ChecksumConst(hrp, data) == bech32Const
}

// bech32ChecksumConst returns the polymod of hrp + data (data includes the
// 6 checksum symbols): bech32Const for a valid Bech32 string, bech32mConst
// for a valid Bech32m string, anything else for an invalid checksum.
func bech32ChecksumConst(hrp string, data []int) int {
	values := append(bech32HRPExpand(hrp), data...)
	return bech32Polymod(values)
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

// Bech32Decode decodes a Bech32 string, returning the HRP and data. Bech32m
// strings are rejected; use DecodeBech32Address for addresses, which accepts
// whichever encoding the witness version requires.
func Bech32Decode(bech string) (string, []int, error) {
	hrp, data, checksumConst, err := bech32DecodeAny(bech)
	if err != nil {
		return "", nil, err
	}
	if checksumConst != bech32Const {
		return "", nil, fmt.Errorf("invalid bech32 checksum")
	}
	return hrp, data, nil
}

// bech32DecodeAny decodes a Bech32 or Bech32m string. It returns the HRP, the
// data without the checksum, and which checksum constant matched
// (bech32Const or bech32mConst).
func bech32DecodeAny(bech string) (string, []int, int, error) {
	if len(bech) > 90 {
		return "", nil, 0, fmt.Errorf("bech32 string too long")
	}

	lower := strings.ToLower(bech)
	upper := strings.ToUpper(bech)
	if bech != lower && bech != upper {
		return "", nil, 0, fmt.Errorf("mixed case in bech32 string")
	}
	bech = lower

	pos := strings.LastIndex(bech, "1")
	if pos < 1 || pos+7 > len(bech) {
		return "", nil, 0, fmt.Errorf("invalid bech32 separator position")
	}

	hrp := bech[:pos]
	dataStr := bech[pos+1:]

	data := make([]int, len(dataStr))
	for i, c := range dataStr {
		if c > 127 || bech32CharsetRev[c] == -1 {
			return "", nil, 0, fmt.Errorf("invalid bech32 character: %c", c)
		}
		data[i] = int(bech32CharsetRev[c])
	}

	checksumConst := bech32ChecksumConst(hrp, data)
	if checksumConst != bech32Const && checksumConst != bech32mConst {
		return "", nil, 0, fmt.Errorf("invalid bech32 checksum")
	}

	return hrp, data[:len(data)-6], checksumConst, nil
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

// DecodeBech32Address decodes a SegWit address and returns the witness version
// and witness program bytes. It enforces the BIP173/BIP350 address rules:
//   - witness version 0 must use the Bech32 checksum, versions 1-16 must use
//     Bech32m (an address with the other checksum is rejected, even though the
//     checksum itself verifies);
//   - witness version is at most 16;
//   - the program is 2 to 40 bytes, and exactly 20 or 32 bytes for version 0.
func DecodeBech32Address(address, expectedHRP string) (byte, []byte, error) {
	hrp, data, checksumConst, err := bech32DecodeAny(address)
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
	if witnessVersion > 16 {
		return 0, nil, fmt.Errorf("invalid witness version: %d", witnessVersion)
	}
	if witnessVersion == 0 && checksumConst != bech32Const {
		return 0, nil, fmt.Errorf("witness version 0 address must use the bech32 checksum, not bech32m")
	}
	if witnessVersion != 0 && checksumConst != bech32mConst {
		return 0, nil, fmt.Errorf("witness version %d address must use the bech32m checksum, not bech32", witnessVersion)
	}

	program, err := ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return 0, nil, fmt.Errorf("converting bits: %w", err)
	}

	programBytes := make([]byte, len(program))
	for i, v := range program {
		programBytes[i] = byte(v)
	}

	if len(programBytes) < 2 || len(programBytes) > 40 {
		return 0, nil, fmt.Errorf("invalid witness program length: %d", len(programBytes))
	}
	if witnessVersion == 0 && len(programBytes) != 20 && len(programBytes) != 32 {
		return 0, nil, fmt.Errorf("invalid witness program length for v0: %d", len(programBytes))
	}

	return witnessVersion, programBytes, nil
}

// segwitOutputScript returns the output script for a decoded SegWit address,
// for the address types GSE can pay: P2WPKH and P2WSH (witness version 0) and
// P2TR (witness version 1 with a 32-byte program). Anything else is an error.
// ValidateAddress and AddressToScript both use it, so an address is accepted
// exactly when it can be paid.
func segwitOutputScript(witnessVersion byte, program []byte) ([]byte, error) {
	switch {
	case witnessVersion == 0 && len(program) == 20:
		return coinbase.P2WPKHScript(program), nil
	case witnessVersion == 0 && len(program) == 32:
		return coinbase.P2WSHScript(program), nil
	case witnessVersion == 1 && len(program) == 32:
		return coinbase.P2TRScript(program), nil
	}
	return nil, fmt.Errorf("unsupported witness version %d with %d-byte program", witnessVersion, len(program))
}
