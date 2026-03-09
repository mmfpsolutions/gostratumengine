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

// GenericCoin implements the Coin interface for user-defined SHA256d coins.
// It is configured via a CoinDefinition provided in config.json.
type GenericCoin struct {
	definition CoinDefinition
	coinType   string
}

// NewGenericCoin creates a new GenericCoin from a coin definition.
func NewGenericCoin(coinType string, def CoinDefinition) *GenericCoin {
	return &GenericCoin{
		definition: def,
		coinType:   coinType,
	}
}

func (g *GenericCoin) Name() string      { return g.definition.Name }
func (g *GenericCoin) Symbol() string    { return g.definition.Symbol }
func (g *GenericCoin) Algorithm() string { return "sha256d" }
func (g *GenericCoin) SupportsSegWit() bool { return g.definition.Segwit }

func (g *GenericCoin) Params() CoinParams {
	p := CoinParams{
		P2PKHVersionMainnet: byte(g.definition.Address.Base58.P2PKH.Mainnet),
		P2PKHVersionTestnet: byte(g.definition.Address.Base58.P2PKH.Testnet),
		SegWit:              g.definition.Segwit,
	}
	if g.definition.Address.Base58.P2SH != nil {
		p.P2SHVersionMainnet = byte(g.definition.Address.Base58.P2SH.Mainnet)
		p.P2SHVersionTestnet = byte(g.definition.Address.Base58.P2SH.Testnet)
	}
	if g.definition.Segwit && g.definition.Address.Bech32 != nil {
		p.Bech32HRPMainnet = g.definition.Address.Bech32.HRP.Mainnet
		p.Bech32HRPTestnet = g.definition.Address.Bech32.HRP.Testnet
	}
	return p
}

func (g *GenericCoin) TemplateRules() []string {
	if g.definition.Segwit {
		return []string{"segwit"}
	}
	return []string{}
}

func (g *GenericCoin) ValidateAddress(address, network string) error {
	params := g.Params()
	hrp := params.Bech32HRPMainnet
	p2pkhVersion := params.P2PKHVersionMainnet
	p2shVersion := params.P2SHVersionMainnet

	if network == "testnet" {
		hrp = params.Bech32HRPTestnet
		p2pkhVersion = params.P2PKHVersionTestnet
		p2shVersion = params.P2SHVersionTestnet
	}

	// Try Bech32 if SegWit is supported
	if g.definition.Segwit && hrp != "" {
		if _, _, err := DecodeBech32Address(address, hrp); err == nil {
			return nil
		}
	}

	// Try Base58Check
	version, payload, err := Base58CheckDecode(address)
	if err != nil {
		return fmt.Errorf("invalid %s address: %s", g.definition.Name, address)
	}
	if len(payload) != 20 {
		return fmt.Errorf("invalid %s address payload length: %d", g.definition.Name, len(payload))
	}
	if version != p2pkhVersion && version != p2shVersion {
		return fmt.Errorf("invalid %s address version byte: 0x%02x", g.definition.Name, version)
	}

	return nil
}

func (g *GenericCoin) AddressToScript(address, network string) ([]byte, error) {
	params := g.Params()
	hrp := params.Bech32HRPMainnet
	p2pkhVersion := params.P2PKHVersionMainnet
	p2shVersion := params.P2SHVersionMainnet

	if network == "testnet" {
		hrp = params.Bech32HRPTestnet
		p2pkhVersion = params.P2PKHVersionTestnet
		p2shVersion = params.P2SHVersionTestnet
	}

	// Try Bech32 if SegWit is supported
	if g.definition.Segwit && hrp != "" {
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

func (g *GenericCoin) BuildCoinbase(template *noderpc.BlockTemplate, address, network, coinbaseText string,
	extraNonce1Size, extraNonce2Size int, extraOutputs []coinbase.CoinbaseOutput) (string, string, error) {

	script, err := g.AddressToScript(address, network)
	if err != nil {
		return "", "", fmt.Errorf("building output script: %w", err)
	}

	poolValue := template.CoinbaseValue
	for _, eo := range extraOutputs {
		poolValue -= eo.Value
	}
	outputs := []coinbase.CoinbaseOutput{
		{Value: poolValue, Script: script},
	}

	outputs = append(outputs, extraOutputs...)

	// Add witness commitment if present
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
		outputs, g.definition.Segwit,
	)

	return coinb1, coinb2, nil
}

func (g *GenericCoin) BuildBlock(header []byte, coinbaseTx []byte, template *noderpc.BlockTemplate) (string, error) {
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

func (g *GenericCoin) PoolReward(template *noderpc.BlockTemplate) int64 {
	return template.CoinbaseValue
}
