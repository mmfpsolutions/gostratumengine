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
	"crypto/sha256"
	"errors"
	"math/big"
)

// Base58 encoding/decoding with checksum (Base58Check) for legacy Bitcoin addresses.

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base58AlphabetRev [256]int

func init() {
	for i := range base58AlphabetRev {
		base58AlphabetRev[i] = -1
	}
	for i, c := range base58Alphabet {
		base58AlphabetRev[c] = i
	}
}

// Base58Decode decodes a Base58-encoded string to bytes.
func Base58Decode(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("empty base58 string")
	}

	result := big.NewInt(0)
	base := big.NewInt(58)

	for _, c := range s {
		if c > 255 || base58AlphabetRev[c] == -1 {
			return nil, errors.New("invalid base58 character")
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(base58AlphabetRev[c])))
	}

	b := result.Bytes()

	// Count leading '1's which represent leading zero bytes
	var leadingZeros int
	for _, c := range s {
		if c != '1' {
			break
		}
		leadingZeros++
	}

	decoded := make([]byte, leadingZeros+len(b))
	copy(decoded[leadingZeros:], b)

	return decoded, nil
}

// Base58CheckDecode decodes a Base58Check-encoded string, verifying the checksum.
// Returns the version byte and payload.
func Base58CheckDecode(address string) (byte, []byte, error) {
	decoded, err := Base58Decode(address)
	if err != nil {
		return 0, nil, err
	}

	if len(decoded) < 5 {
		return 0, nil, errors.New("base58check decoded data too short")
	}

	// Last 4 bytes are the checksum
	payload := decoded[:len(decoded)-4]
	checksum := decoded[len(decoded)-4:]

	// Verify checksum
	hash := sha256.Sum256(payload)
	hash = sha256.Sum256(hash[:])

	if hash[0] != checksum[0] || hash[1] != checksum[1] ||
		hash[2] != checksum[2] || hash[3] != checksum[3] {
		return 0, nil, errors.New("invalid base58check checksum")
	}

	return payload[0], payload[1:], nil
}
