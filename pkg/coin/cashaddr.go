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

// CashAddr encoding/decoding for Bitcoin Cash and eCash addresses.

const cashAddrCharset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

var cashAddrCharsetRev [128]int8

func init() {
	for i := range cashAddrCharsetRev {
		cashAddrCharsetRev[i] = -1
	}
	for i, c := range cashAddrCharset {
		cashAddrCharsetRev[c] = int8(i)
	}
}

func cashAddrPolymod(values []uint64) uint64 {
	gen := [5]uint64{
		0x98f2bc8e61, 0x79b76d99e2, 0xf33e5fb3c4, 0xae2eabe2a8, 0x1e4f43e470,
	}
	c := uint64(1)
	for _, v := range values {
		c0 := c >> 35
		c = ((c & 0x07ffffffff) << 5) ^ v
		for i := 0; i < 5; i++ {
			if (c0>>uint(i))&1 != 0 {
				c ^= gen[i]
			}
		}
	}
	return c ^ 1
}

func cashAddrPrefixExpand(prefix string) []uint64 {
	ret := make([]uint64, 0, len(prefix)+1)
	for _, c := range prefix {
		ret = append(ret, uint64(c&0x1f))
	}
	ret = append(ret, 0)
	return ret
}

// CashAddrDecode decodes a CashAddr address and returns the hash type and hash bytes.
// Accepts addresses with or without the prefix (e.g., "bitcoincash:qr..." or "qr...").
func CashAddrDecode(address, expectedPrefix string) (byte, []byte, error) {
	lower := strings.ToLower(address)

	// Add prefix if missing
	if !strings.Contains(lower, ":") {
		lower = expectedPrefix + ":" + lower
	}

	parts := strings.SplitN(lower, ":", 2)
	if len(parts) != 2 {
		return 0, nil, fmt.Errorf("invalid cashaddr format")
	}
	prefix := parts[0]
	payload := parts[1]

	if prefix != expectedPrefix {
		return 0, nil, fmt.Errorf("unexpected prefix: got %s, want %s", prefix, expectedPrefix)
	}

	// Decode base32
	data := make([]uint64, len(payload))
	for i, c := range payload {
		if c > 127 || cashAddrCharsetRev[c] == -1 {
			return 0, nil, fmt.Errorf("invalid cashaddr character: %c", c)
		}
		data[i] = uint64(cashAddrCharsetRev[c])
	}

	// Verify checksum (last 8 characters)
	if len(data) < 8 {
		return 0, nil, fmt.Errorf("cashaddr too short")
	}

	prefixData := cashAddrPrefixExpand(prefix)
	values := append(prefixData, data...)
	if cashAddrPolymod(values) != 0 {
		return 0, nil, fmt.Errorf("invalid cashaddr checksum")
	}

	// Remove checksum
	data = data[:len(data)-8]

	// Convert from 5-bit to 8-bit
	converted, err := cashAddrConvertBits(data, 5, 8, false)
	if err != nil {
		return 0, nil, fmt.Errorf("converting bits: %w", err)
	}

	if len(converted) < 1 {
		return 0, nil, fmt.Errorf("empty cashaddr payload")
	}

	// First byte encodes hash type and size
	versionByte := converted[0]
	hashType := versionByte >> 3
	hashSize := cashAddrHashSize(versionByte & 0x07)
	hash := converted[1:]

	if len(hash) != hashSize {
		return 0, nil, fmt.Errorf("cashaddr hash length mismatch: got %d, want %d", len(hash), hashSize)
	}

	return byte(hashType), byteSliceFromUint64(hash), nil
}

func cashAddrHashSize(sizeBits uint64) int {
	switch sizeBits {
	case 0:
		return 20
	case 1:
		return 24
	case 2:
		return 28
	case 3:
		return 32
	case 4:
		return 40
	case 5:
		return 48
	case 6:
		return 56
	case 7:
		return 64
	default:
		return 20
	}
}

func cashAddrConvertBits(data []uint64, fromBits, toBits uint, pad bool) ([]uint64, error) {
	acc := uint64(0)
	bits := uint(0)
	var ret []uint64
	maxv := uint64((1 << toBits) - 1)

	for _, value := range data {
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

func byteSliceFromUint64(data []uint64) []byte {
	result := make([]byte, len(data))
	for i, v := range data {
		result[i] = byte(v)
	}
	return result
}
