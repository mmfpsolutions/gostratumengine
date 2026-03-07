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

	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
)

// DigiByte implements the Coin interface for DGB.
type DigiByte struct{}

func init() {
	Register("digibyte", &DigiByte{})
}

func (d *DigiByte) Name() string      { return "DigiByte" }
func (d *DigiByte) Symbol() string    { return "DGB" }
func (d *DigiByte) Algorithm() string { return "sha256d" }
func (d *DigiByte) SupportsSegWit() bool { return true }

func (d *DigiByte) Params() CoinParams {
	return CoinParams{
		P2PKHVersionMainnet: 0x1E,
		P2PKHVersionTestnet: 0x7E,
		P2SHVersionMainnet:  0x3F,
		P2SHVersionTestnet:  0x8C,
		Bech32HRPMainnet:    "dgb",
		Bech32HRPTestnet:    "dgbt",
		DefaultRPCPort:      14022,
		SegWit:              true,
	}
}

func (d *DigiByte) TemplateRules() []string {
	return []string{"segwit"}
}

func (d *DigiByte) ValidateAddress(address, network string) error {
	params := d.Params()
	hrp := params.Bech32HRPMainnet
	p2pkhVersion := params.P2PKHVersionMainnet
	p2shVersion := params.P2SHVersionMainnet

	if network == "testnet" {
		hrp = params.Bech32HRPTestnet
		p2pkhVersion = params.P2PKHVersionTestnet
		p2shVersion = params.P2SHVersionTestnet
	}

	// Try Bech32
	if _, _, err := DecodeBech32Address(address, hrp); err == nil {
		return nil
	}

	// Try Base58Check
	version, payload, err := Base58CheckDecode(address)
	if err != nil {
		return fmt.Errorf("invalid DigiByte address: %s", address)
	}
	if len(payload) != 20 {
		return fmt.Errorf("invalid DigiByte address payload length: %d", len(payload))
	}
	if version != p2pkhVersion && version != p2shVersion {
		return fmt.Errorf("invalid DigiByte address version byte: 0x%02x", version)
	}

	return nil
}

func (d *DigiByte) addressToScript(address, network string) ([]byte, error) {
	params := d.Params()
	hrp := params.Bech32HRPMainnet
	p2pkhVersion := params.P2PKHVersionMainnet
	p2shVersion := params.P2SHVersionMainnet

	if network == "testnet" {
		hrp = params.Bech32HRPTestnet
		p2pkhVersion = params.P2PKHVersionTestnet
		p2shVersion = params.P2SHVersionTestnet
	}

	// Try Bech32
	witnessVersion, program, err := DecodeBech32Address(address, hrp)
	if err == nil {
		if witnessVersion == 0 && len(program) == 20 {
			return coinbase.P2WPKHScript(program), nil
		}
		if witnessVersion == 0 && len(program) == 32 {
			return coinbase.P2WSHScript(program), nil
		}
		return nil, fmt.Errorf("unsupported witness version %d", witnessVersion)
	}

	// Try Base58Check
	version, payload, err := Base58CheckDecode(address)
	if err != nil {
		return nil, fmt.Errorf("cannot decode address: %s", address)
	}
	if version == p2pkhVersion {
		return coinbase.P2PKHScript(payload), nil
	}
	if version == p2shVersion {
		return coinbase.P2SHScript(payload), nil
	}

	return nil, fmt.Errorf("unknown address version: 0x%02x", version)
}

func (d *DigiByte) BuildCoinbase(template *noderpc.BlockTemplate, address, network, coinbaseText string,
	extraNonce1Size, extraNonce2Size int) (string, string, error) {

	script, err := d.addressToScript(address, network)
	if err != nil {
		return "", "", fmt.Errorf("building output script: %w", err)
	}

	outputs := []coinbase.CoinbaseOutput{
		{Value: template.CoinbaseValue, Script: script},
	}

	// Add witness commitment if present (included in txid format for merkle root correctness)
	if template.DefaultWitnessCommitment != "" {
		commitment, err := hex.DecodeString(template.DefaultWitnessCommitment)
		if err != nil {
			return "", "", fmt.Errorf("decoding witness commitment: %w", err)
		}
		outputs = append(outputs, coinbase.CoinbaseOutput{
			Value:  0,
			Script: commitment,
		})
	}

	coinb1, coinb2 := coinbase.BuildCoinbaseParts(
		template.Height, coinbaseText,
		extraNonce1Size, extraNonce2Size,
		outputs, true,
	)

	return coinb1, coinb2, nil
}

func (d *DigiByte) BuildBlock(header []byte, coinbaseTx []byte, template *noderpc.BlockTemplate) (string, error) {
	// For SegWit blocks, add witness marker/flag and witness data to the coinbase
	// The coinbase from the miner is in txid format; we need the full witness format for the block
	if template.DefaultWitnessCommitment != "" {
		coinbaseTx = coinbase.AddWitnessData(coinbaseTx)
	}

	var block []byte
	block = append(block, header...)
	block = append(block, coinbase.SerializeVarInt(uint64(len(template.Transactions)+1))...)
	block = append(block, coinbaseTx...)

	for _, tx := range template.Transactions {
		txData, err := hex.DecodeString(tx.Data)
		if err != nil {
			return "", fmt.Errorf("decoding transaction: %w", err)
		}
		block = append(block, txData...)
	}

	return hex.EncodeToString(block), nil
}

func (d *DigiByte) PoolReward(template *noderpc.BlockTemplate) int64 {
	return template.CoinbaseValue
}
