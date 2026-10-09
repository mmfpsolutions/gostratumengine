/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package engine

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/mmfpsolutions/gostratumengine/pkg/coin"
)

// coinTypeForSymbol maps an AUTHORS symbol to its registered coin type.
var coinTypeForSymbol = map[string]string{
	"BTC": "bitcoin",
	"BCH": "bitcoincash",
	"DGB": "digibyte",
	"XEC": "ecash",
	"BC2": "bitcoinii",
}

// btcsDefinition is the coin_definition an operator needs to mine Bitcoin
// Silver, which isn't a built-in coin: P2PKH 26, P2SH 5, Bech32 prefix "bs".
var btcsDefinition = coin.CoinDefinition{
	Name:   "Bitcoin Silver",
	Symbol: "BTCS",
	Segwit: true,
	Address: coin.AddressConfig{
		Base58: &coin.Base58Config{
			P2PKH: coin.NetworkVersions{Mainnet: 26, Testnet: 111},
			P2SH:  &coin.NetworkVersions{Mainnet: 5, Testnet: 196},
		},
		Bech32: &coin.Bech32Config{HRP: coin.NetworkHRP{Mainnet: "bs", Testnet: "tbs"}},
	},
}

// coinForSymbol returns the coin an AUTHORS symbol is paid through: a
// built-in coin, or for BTCS a generic coin built from its definition.
func coinForSymbol(symbol string) (coin.Coin, error) {
	if symbol == "BTCS" {
		return coin.NewGenericCoin("bitcoinsilver", btcsDefinition), nil
	}
	coinType, ok := coinTypeForSymbol[symbol]
	if !ok {
		return nil, fmt.Errorf("AUTHORS has symbol %s with no coin in this test: add it", symbol)
	}
	return coin.Get(coinType)
}

// Every AUTHORS entry must resolve to an output script. A bad address doesn't
// stop the pool: the donation is silently disabled with a warning, so only a
// test catches it.
func TestAuthors_EveryEntryResolvesToAScript(t *testing.T) {
	entries := 0
	for _, line := range strings.Split(authorsFile, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Errorf("malformed AUTHORS line (want SYMBOL NETWORK ADDRESS): %q", line)
			continue
		}
		symbol, network, address := fields[0], fields[1], fields[2]
		entries++

		c, err := coinForSymbol(symbol)
		if err != nil {
			t.Errorf("%s: %v", symbol, err)
			continue
		}
		// What the engine does at startup: look the address up, then convert it.
		got, err := loadDonationAddress(symbol, network)
		if err != nil || got != address {
			t.Errorf("loadDonationAddress(%s, %s) = (%q, %v), want %q", symbol, network, got, err, address)
		}
		script, err := c.AddressToScript(address, network)
		if err != nil {
			t.Errorf("%s %s: address %s does not convert to a script: %v", symbol, network, address, err)
			continue
		}
		if len(script) == 0 {
			t.Errorf("%s %s: empty script for %s", symbol, network, address)
		}
	}
	if entries == 0 {
		t.Fatal("AUTHORS has no entries")
	}
}

// The mainnet donation scripts, byte for byte. The expected hashes were
// decoded from the addresses independently of this codebase (Bech32 and
// CashAddr, 2026-10-08), so a wrong script type or a mis-typed address fails.
func TestAuthors_MainnetScripts(t *testing.T) {
	cases := []struct{ symbol, wantScript string }{
		// P2WPKH: OP_0 PUSH20 <program>
		{"BTC", "0014a83aa9b14d96f1d30fc2c43b391fad592bd6e544"},
		{"DGB", "00148937a12a47f7264df12d7a0fb394c8cc46a08eb1"},
		{"BTCS", "0014c25de71da6d3d6e1aee69f653a96e9e280d46eae"},
		// P2PKH: OP_DUP OP_HASH160 PUSH20 <hash> OP_EQUALVERIFY OP_CHECKSIG
		{"BCH", "76a914cb6a0bc8e0edf700f99f8f858a3dd2e8b3f8626c88ac"},
	}
	for _, c := range cases {
		t.Run(c.symbol, func(t *testing.T) {
			address, err := loadDonationAddress(c.symbol, "mainnet")
			if err != nil {
				t.Fatalf("loadDonationAddress: %v", err)
			}
			cn, err := coinForSymbol(c.symbol)
			if err != nil {
				t.Fatalf("coinForSymbol: %v", err)
			}
			script, err := cn.AddressToScript(address, "mainnet")
			if err != nil {
				t.Fatalf("AddressToScript(%s): %v", address, err)
			}
			if got := hex.EncodeToString(script); got != c.wantScript {
				t.Errorf("%s donation script for %s = %s, want %s", c.symbol, address, got, c.wantScript)
			}
		})
	}
}

func TestLoadDonationAddress_NoEntry(t *testing.T) {
	if _, err := loadDonationAddress("NOPE", "mainnet"); err == nil {
		t.Error("expected an error for a coin with no AUTHORS entry")
	}
	if _, err := loadDonationAddress("XEC", "testnet"); err == nil {
		t.Error("expected an error for XEC testnet, which has no AUTHORS entry")
	}
}
