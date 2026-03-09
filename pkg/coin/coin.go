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
	"regexp"
	"sync"

	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
)

// Coin defines coin-specific behavior for mining operations.
type Coin interface {
	// Name returns the display name (e.g., "Bitcoin").
	Name() string

	// Symbol returns the ticker symbol (e.g., "BTC").
	Symbol() string

	// Algorithm returns the mining algorithm (e.g., "sha256d").
	Algorithm() string

	// ValidateAddress checks whether an address is valid for the given network.
	ValidateAddress(address, network string) error

	// Params returns static network parameters.
	Params() CoinParams

	// TemplateRules returns the rules to pass to getblocktemplate.
	TemplateRules() []string

	// AddressToScript converts a coin address to its output script.
	AddressToScript(address, network string) ([]byte, error)

	// BuildCoinbase constructs the coinbase transaction, split into two halves
	// (coinb1 and coinb2) with space for extranonce1+extranonce2 between them.
	// extraOutputs are appended after the pool output (e.g., donation outputs).
	// Returns hex-encoded halves.
	BuildCoinbase(template *noderpc.BlockTemplate, address, network, coinbaseText string,
		extraNonce1Size, extraNonce2Size int, extraOutputs []coinbase.CoinbaseOutput) (coinb1, coinb2 string, err error)

	// BuildBlock constructs the full block hex for submission from the solved header,
	// the full coinbase transaction, and the block template.
	BuildBlock(header []byte, coinbaseTx []byte, template *noderpc.BlockTemplate) (string, error)

	// PoolReward returns the satoshi value the pool receives from the coinbase.
	// For most coins this equals CoinbaseValue; for eCash it excludes mandatory splits.
	PoolReward(template *noderpc.BlockTemplate) int64

	// SupportsSegWit returns whether this coin uses SegWit.
	SupportsSegWit() bool
}

// CoinParams holds static network parameters for a coin.
type CoinParams struct {
	P2PKHVersionMainnet byte
	P2PKHVersionTestnet byte
	P2SHVersionMainnet  byte
	P2SHVersionTestnet  byte
	Bech32HRPMainnet    string // empty if not supported
	Bech32HRPTestnet    string
	CashAddrPrefix      string // empty if not BCH/XEC
	DefaultRPCPort      int
	SegWit              bool
}

var (
	registry   = map[string]Coin{}
	registryMu sync.RWMutex
)

// Register adds a coin to the global registry by its coin_type key.
func Register(coinType string, c Coin) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[coinType] = c
}

// Get returns a registered coin by its coin_type key.
func Get(coinType string) (Coin, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := registry[coinType]
	if !ok {
		return nil, fmt.Errorf("unknown coin type: %s", coinType)
	}
	return c, nil
}

// List returns all registered coin type keys.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// CoinDefinition describes a user-defined SHA256d coin for generic registration.
// Used inline in config.json when coin_type is not a built-in.
type CoinDefinition struct {
	Name    string        `json:"name"`
	Symbol  string        `json:"symbol"`
	Segwit  bool          `json:"segwit"`
	Address AddressConfig `json:"address"`
}

// AddressConfig defines address formats for a coin definition.
type AddressConfig struct {
	Base58 *Base58Config `json:"base58"`
	Bech32 *Bech32Config `json:"bech32,omitempty"`
}

// Base58Config defines Base58Check address version bytes.
type Base58Config struct {
	P2PKH NetworkVersions  `json:"p2pkh"`
	P2SH  *NetworkVersions `json:"p2sh,omitempty"`
}

// NetworkVersions defines version bytes for mainnet and testnet.
type NetworkVersions struct {
	Mainnet int `json:"mainnet"`
	Testnet int `json:"testnet"`
}

// Bech32Config defines Bech32 address parameters.
type Bech32Config struct {
	HRP NetworkHRP `json:"hrp"`
}

// NetworkHRP defines human-readable parts for mainnet and testnet.
type NetworkHRP struct {
	Mainnet string `json:"mainnet"`
	Testnet string `json:"testnet"`
}

var (
	coinTypeRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	symbolRe   = regexp.MustCompile(`^[A-Z0-9]+$`)
)

// ValidateCoinDefinition validates a user-provided coin definition.
func ValidateCoinDefinition(coinType string, def *CoinDefinition) error {
	if !coinTypeRe.MatchString(coinType) {
		return fmt.Errorf("coin_type %q must be lowercase alphanumeric", coinType)
	}
	if def.Name == "" {
		return fmt.Errorf("coin_type %q: name is required", coinType)
	}
	if len(def.Symbol) < 2 || len(def.Symbol) > 10 || !symbolRe.MatchString(def.Symbol) {
		return fmt.Errorf("coin_type %q: symbol must be 2-10 uppercase characters, got %q", coinType, def.Symbol)
	}
	if def.Address.Base58 == nil {
		return fmt.Errorf("coin_type %q: address.base58 is required", coinType)
	}
	if err := validateVersionByte(def.Address.Base58.P2PKH.Mainnet, coinType, "p2pkh.mainnet"); err != nil {
		return err
	}
	if err := validateVersionByte(def.Address.Base58.P2PKH.Testnet, coinType, "p2pkh.testnet"); err != nil {
		return err
	}
	if def.Address.Base58.P2SH != nil {
		if err := validateVersionByte(def.Address.Base58.P2SH.Mainnet, coinType, "p2sh.mainnet"); err != nil {
			return err
		}
		if err := validateVersionByte(def.Address.Base58.P2SH.Testnet, coinType, "p2sh.testnet"); err != nil {
			return err
		}
	}
	if def.Segwit {
		if def.Address.Bech32 == nil {
			return fmt.Errorf("coin_type %q: address.bech32 is required when segwit is true", coinType)
		}
		if def.Address.Bech32.HRP.Mainnet == "" {
			return fmt.Errorf("coin_type %q: bech32 hrp mainnet is required", coinType)
		}
		if def.Address.Bech32.HRP.Testnet == "" {
			return fmt.Errorf("coin_type %q: bech32 hrp testnet is required", coinType)
		}
	}
	return nil
}

func validateVersionByte(v int, coinType, field string) error {
	if v < 0 || v > 255 {
		return fmt.Errorf("coin_type %q: %s must be 0-255, got %d", coinType, field, v)
	}
	return nil
}
