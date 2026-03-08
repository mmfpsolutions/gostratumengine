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

// BitcoinCash implements the Coin interface for BCH.
type BitcoinCash struct{}

func init() {
	Register("bitcoincash", &BitcoinCash{})
}

func (b *BitcoinCash) Name() string      { return "Bitcoin Cash" }
func (b *BitcoinCash) Symbol() string    { return "BCH" }
func (b *BitcoinCash) Algorithm() string { return "sha256d" }
func (b *BitcoinCash) SupportsSegWit() bool { return false }

func (b *BitcoinCash) Params() CoinParams {
	return CoinParams{
		P2PKHVersionMainnet: 0x00,
		P2PKHVersionTestnet: 0x6F,
		P2SHVersionMainnet:  0x05,
		P2SHVersionTestnet:  0xC4,
		CashAddrPrefix:      "bitcoincash",
		DefaultRPCPort:      8332,
		SegWit:              false,
	}
}

func (b *BitcoinCash) TemplateRules() []string {
	return []string{}
}

func (b *BitcoinCash) ValidateAddress(address, network string) error {
	prefix := b.Params().CashAddrPrefix
	if network == "testnet" {
		prefix = "bchtest"
	}

	// Try CashAddr
	hashType, hash, err := CashAddrDecode(address, prefix)
	if err == nil {
		if hashType > 1 {
			return fmt.Errorf("unsupported BCH address hash type: %d", hashType)
		}
		if len(hash) != 20 {
			return fmt.Errorf("invalid BCH address hash length: %d", len(hash))
		}
		return nil
	}

	// Try legacy Base58Check
	p2pkhVersion := b.Params().P2PKHVersionMainnet
	p2shVersion := b.Params().P2SHVersionMainnet
	if network == "testnet" {
		p2pkhVersion = b.Params().P2PKHVersionTestnet
		p2shVersion = b.Params().P2SHVersionTestnet
	}

	version, payload, err := Base58CheckDecode(address)
	if err != nil {
		return fmt.Errorf("invalid Bitcoin Cash address: %s", address)
	}
	if len(payload) != 20 {
		return fmt.Errorf("invalid BCH address payload length: %d", len(payload))
	}
	if version != p2pkhVersion && version != p2shVersion {
		return fmt.Errorf("invalid BCH address version byte: 0x%02x", version)
	}

	return nil
}

func (b *BitcoinCash) AddressToScript(address, network string) ([]byte, error) {
	prefix := b.Params().CashAddrPrefix
	if network == "testnet" {
		prefix = "bchtest"
	}

	// Try CashAddr
	hashType, hash, err := CashAddrDecode(address, prefix)
	if err == nil {
		if hashType == 0 {
			return coinbase.P2PKHScript(hash), nil
		}
		if hashType == 1 {
			return coinbase.P2SHScript(hash), nil
		}
		return nil, fmt.Errorf("unsupported BCH hash type: %d", hashType)
	}

	// Try legacy Base58Check
	p2pkhVersion := b.Params().P2PKHVersionMainnet
	p2shVersion := b.Params().P2SHVersionMainnet
	if network == "testnet" {
		p2pkhVersion = b.Params().P2PKHVersionTestnet
		p2shVersion = b.Params().P2SHVersionTestnet
	}

	version, payload, bErr := Base58CheckDecode(address)
	if bErr != nil {
		return nil, fmt.Errorf("cannot decode BCH address: %s", address)
	}
	if version == p2pkhVersion {
		return coinbase.P2PKHScript(payload), nil
	}
	if version == p2shVersion {
		return coinbase.P2SHScript(payload), nil
	}

	return nil, fmt.Errorf("unknown BCH address version: 0x%02x", version)
}

func (b *BitcoinCash) BuildCoinbase(template *noderpc.BlockTemplate, address, network, coinbaseText string,
	extraNonce1Size, extraNonce2Size int, extraOutputs []coinbase.CoinbaseOutput) (string, string, error) {

	script, err := b.AddressToScript(address, network)
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

	// No SegWit for BCH
	coinb1, coinb2 := coinbase.BuildCoinbaseParts(
		template.Height, coinbaseText,
		extraNonce1Size, extraNonce2Size,
		outputs, false,
	)

	return coinb1, coinb2, nil
}

func (b *BitcoinCash) BuildBlock(header []byte, coinbaseTx []byte, template *noderpc.BlockTemplate) (string, error) {
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

func (b *BitcoinCash) PoolReward(template *noderpc.BlockTemplate) int64 {
	return template.CoinbaseValue
}
