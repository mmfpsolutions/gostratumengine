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

// ECash implements the Coin interface for XEC (eCash).
type ECash struct{}

func init() {
	Register("ecash", &ECash{})
}

func (e *ECash) Name() string      { return "eCash" }
func (e *ECash) Symbol() string    { return "XEC" }
func (e *ECash) Algorithm() string { return "sha256d" }
func (e *ECash) SupportsSegWit() bool { return false }

func (e *ECash) Params() CoinParams {
	return CoinParams{
		P2PKHVersionMainnet: 0x00,
		P2PKHVersionTestnet: 0x6F,
		P2SHVersionMainnet:  0x05,
		P2SHVersionTestnet:  0xC4,
		CashAddrPrefix:      "ecash",
		DefaultRPCPort:      8332,
		SegWit:              false,
	}
}

func (e *ECash) TemplateRules() []string {
	return []string{}
}

func (e *ECash) ValidateAddress(address, network string) error {
	prefix := e.Params().CashAddrPrefix
	if network == "testnet" {
		prefix = "ectest"
	}

	// Try CashAddr
	hashType, hash, err := CashAddrDecode(address, prefix)
	if err == nil {
		if hashType > 1 {
			return fmt.Errorf("unsupported eCash address hash type: %d", hashType)
		}
		if len(hash) != 20 {
			return fmt.Errorf("invalid eCash address hash length: %d", len(hash))
		}
		return nil
	}

	// Try legacy Base58Check
	p2pkhVersion := e.Params().P2PKHVersionMainnet
	p2shVersion := e.Params().P2SHVersionMainnet
	if network == "testnet" {
		p2pkhVersion = e.Params().P2PKHVersionTestnet
		p2shVersion = e.Params().P2SHVersionTestnet
	}

	version, payload, err := Base58CheckDecode(address)
	if err != nil {
		return fmt.Errorf("invalid eCash address: %s", address)
	}
	if len(payload) != 20 {
		return fmt.Errorf("invalid eCash address payload length: %d", len(payload))
	}
	if version != p2pkhVersion && version != p2shVersion {
		return fmt.Errorf("invalid eCash address version byte: 0x%02x", version)
	}

	return nil
}

func (e *ECash) AddressToScript(address, network string) ([]byte, error) {
	prefix := e.Params().CashAddrPrefix
	if network == "testnet" {
		prefix = "ectest"
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
		return nil, fmt.Errorf("unsupported eCash hash type: %d", hashType)
	}

	// Try legacy Base58Check
	p2pkhVersion := e.Params().P2PKHVersionMainnet
	if network == "testnet" {
		p2pkhVersion = e.Params().P2PKHVersionTestnet
	}

	version, payload, bErr := Base58CheckDecode(address)
	if bErr != nil {
		return nil, fmt.Errorf("cannot decode eCash address: %s", address)
	}
	if version == p2pkhVersion {
		return coinbase.P2PKHScript(payload), nil
	}

	return nil, fmt.Errorf("unknown eCash address version: 0x%02x", version)
}

func (e *ECash) BuildCoinbase(template *noderpc.BlockTemplate, address, network, coinbaseText string,
	extraNonce1Size, extraNonce2Size int, extraOutputs []coinbase.CoinbaseOutput) (string, string, error) {

	poolScript, err := e.AddressToScript(address, network)
	if err != nil {
		return "", "", fmt.Errorf("building pool output script: %w", err)
	}

	// Calculate pool reward after mandatory deductions
	poolReward := e.PoolReward(template)
	for _, eo := range extraOutputs {
		poolReward -= eo.Value
	}

	outputs := []coinbase.CoinbaseOutput{
		{Value: poolReward, Script: poolScript},
	}

	// Add miner fund output if required by the template (via CoinbaseTxn wrapper)
	if template.CoinbaseTxn != nil && template.CoinbaseTxn.MinerFund != nil &&
		len(template.CoinbaseTxn.MinerFund.Addresses) > 0 && template.CoinbaseTxn.MinerFund.MinimumValue > 0 {
		fundScript, err := e.AddressToScript(template.CoinbaseTxn.MinerFund.Addresses[0], network)
		if err != nil {
			return "", "", fmt.Errorf("building miner fund script: %w", err)
		}
		outputs = append(outputs, coinbase.CoinbaseOutput{
			Value:  template.CoinbaseTxn.MinerFund.MinimumValue,
			Script: fundScript,
		})
	}

	// Add staking rewards output if required by the template
	if template.CoinbaseTxn != nil && template.CoinbaseTxn.StakingRewards != nil &&
		template.CoinbaseTxn.StakingRewards.MinimumValue > 0 {
		if template.CoinbaseTxn.StakingRewards.PayoutScript.Hex != "" {
			stakingScript, err := hex.DecodeString(template.CoinbaseTxn.StakingRewards.PayoutScript.Hex)
			if err != nil {
				return "", "", fmt.Errorf("decoding staking rewards script: %w", err)
			}
			outputs = append(outputs, coinbase.CoinbaseOutput{
				Value:  template.CoinbaseTxn.StakingRewards.MinimumValue,
				Script: stakingScript,
			})
		}
	}

	outputs = append(outputs, extraOutputs...)

	// No SegWit for eCash
	coinb1, coinb2 := coinbase.BuildCoinbaseParts(
		template.Height, coinbaseText,
		extraNonce1Size, extraNonce2Size,
		outputs, false,
	)

	return coinb1, coinb2, nil
}

func (e *ECash) BuildBlock(header []byte, coinbaseTx []byte, template *noderpc.BlockTemplate) (string, error) {
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

// PoolReward calculates the pool's share of the coinbase after mandatory deductions.
func (e *ECash) PoolReward(template *noderpc.BlockTemplate) int64 {
	reward := template.CoinbaseValue
	if template.CoinbaseTxn != nil {
		if template.CoinbaseTxn.MinerFund != nil {
			reward -= template.CoinbaseTxn.MinerFund.MinimumValue
		}
		if template.CoinbaseTxn.StakingRewards != nil {
			reward -= template.CoinbaseTxn.StakingRewards.MinimumValue
		}
	}
	if reward < 0 {
		reward = 0
	}
	return reward
}
