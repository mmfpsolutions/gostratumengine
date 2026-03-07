/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package coinbase

import (
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/ripemd160"
)

// Opcodes used in output scripts.
const (
	OpDup         = 0x76
	OpHash160     = 0xa9
	OpEqualVerify = 0x88
	OpCheckSig    = 0xac
	OpEqual       = 0x87
	OpReturn      = 0x6a
	Op0           = 0x00
)

// P2PKHScript creates a Pay-to-Public-Key-Hash output script.
// Input: 20-byte public key hash.
func P2PKHScript(pubKeyHash []byte) []byte {
	script := make([]byte, 0, 25)
	script = append(script, OpDup, OpHash160, 0x14)
	script = append(script, pubKeyHash...)
	script = append(script, OpEqualVerify, OpCheckSig)
	return script
}

// P2SHScript creates a Pay-to-Script-Hash output script.
// Input: 20-byte script hash.
func P2SHScript(scriptHash []byte) []byte {
	script := make([]byte, 0, 23)
	script = append(script, OpHash160, 0x14)
	script = append(script, scriptHash...)
	script = append(script, OpEqual)
	return script
}

// P2WPKHScript creates a Pay-to-Witness-Public-Key-Hash output script (SegWit v0).
// Input: 20-byte public key hash.
func P2WPKHScript(pubKeyHash []byte) []byte {
	script := make([]byte, 0, 22)
	script = append(script, Op0, 0x14)
	script = append(script, pubKeyHash...)
	return script
}

// P2WSHScript creates a Pay-to-Witness-Script-Hash output script (SegWit v0).
// Input: 32-byte script hash.
func P2WSHScript(scriptHash []byte) []byte {
	script := make([]byte, 0, 34)
	script = append(script, Op0, 0x20)
	script = append(script, scriptHash...)
	return script
}

// OpReturnScript creates an OP_RETURN output script with the given data.
func OpReturnScript(data []byte) []byte {
	script := make([]byte, 0, 2+len(data))
	script = append(script, OpReturn)
	if len(data) <= 75 {
		script = append(script, byte(len(data)))
	} else {
		// Use OP_PUSHDATA1 for larger payloads
		script = append(script, 0x4c, byte(len(data)))
	}
	script = append(script, data...)
	return script
}

// Hash160 performs SHA256 followed by RIPEMD160.
func Hash160(data []byte) []byte {
	sha := sha256.Sum256(data)
	r := ripemd160.New()
	r.Write(sha[:])
	return r.Sum(nil)
}

// SerializeVarInt encodes an integer as a Bitcoin variable-length integer.
func SerializeVarInt(n uint64) []byte {
	switch {
	case n < 0xFD:
		return []byte{byte(n)}
	case n <= 0xFFFF:
		return []byte{0xFD, byte(n), byte(n >> 8)}
	case n <= 0xFFFFFFFF:
		return []byte{0xFE, byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}
	default:
		return []byte{0xFF, byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24),
			byte(n >> 32), byte(n >> 40), byte(n >> 48), byte(n >> 56)}
	}
}

// SerializeHeight encodes a block height for the coinbase scriptSig (BIP34).
func SerializeHeight(height int64) []byte {
	if height < 0 {
		return []byte{0x01, 0x00}
	}
	if height == 0 {
		return []byte{0x01, 0x00}
	}

	// Determine number of bytes needed
	h := height
	var buf []byte
	for h > 0 {
		buf = append(buf, byte(h&0xFF))
		h >>= 8
	}
	// If the high bit is set, add a 0x00 byte to keep it positive
	if buf[len(buf)-1]&0x80 != 0 {
		buf = append(buf, 0x00)
	}

	result := make([]byte, 0, 1+len(buf))
	result = append(result, byte(len(buf)))
	result = append(result, buf...)
	return result
}

// PutUint32LE writes a uint32 as 4 little-endian bytes.
func PutUint32LE(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

// PutUint64LE writes a uint64 as 8 little-endian bytes.
func PutUint64LE(v uint64) []byte {
	return []byte{
		byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24),
		byte(v >> 32), byte(v >> 40), byte(v >> 48), byte(v >> 56),
	}
}

// DoubleSHA256 computes SHA256(SHA256(data)).
func DoubleSHA256(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// ReverseBytes returns a new slice with bytes in reverse order.
func ReverseBytes(b []byte) []byte {
	reversed := make([]byte, len(b))
	for i, v := range b {
		reversed[len(b)-1-i] = v
	}
	return reversed
}

// BuildScriptFromAddress is a placeholder that will be filled by coin-specific implementations.
// Each coin provides its own address-to-script logic via the Coin interface.
func BuildScriptFromAddress(address string) ([]byte, error) {
	return nil, fmt.Errorf("use coin-specific address-to-script conversion")
}
