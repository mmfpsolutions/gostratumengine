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
	"strings"
	"testing"
)

// These tests check output SCRIPTS byte for byte. A wrong script still makes a
// valid block, but pays the reward where nobody can spend it. Expected values
// come from the BIP173/BIP350 documents and from an independent reference
// decoder, not from this package.

// segwitScriptHex is the scriptPubKey a witness program encodes to:
// OP_n PUSH<len> <program> (OP_0 = 0x00, OP_1..OP_16 = 0x51..0x60).
func segwitScriptHex(version byte, program []byte) string {
	op := byte(0x00)
	if version > 0 {
		op = 0x50 + version
	}
	return hex.EncodeToString(append([]byte{op, byte(len(program))}, program...))
}

// The valid address vectors from BIP173 and BIP350. Every one must decode to
// exactly the scriptPubKey the BIP lists.
func TestDecodeBech32Address_BIPValidVectors(t *testing.T) {
	cases := []struct{ addr, wantScript string }{
		{"BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4", "0014751e76e8199196d454941c45d1b3a323f1433bd6"},
		{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", "00201863143c14c5166804bd19203356da136c985678cd4d27a1b8c6329604903262"},
		{"bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7kt5nd6y", "5128751e76e8199196d454941c45d1b3a323f1433bd6751e76e8199196d454941c45d1b3a323f1433bd6"},
		{"BC1SW50QGDZ25J", "6002751e"},
		{"bc1zw508d6qejxtdg4y5r3zarvaryvaxxpcs", "5210751e76e8199196d454941c45d1b3a323"},
		{"tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy", "0020000000c4a5cad46221b2a187905e5266362b99d5e91c6ce24d165dab93e86433"},
		{"tb1pqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesf3hn0c", "5120000000c4a5cad46221b2a187905e5266362b99d5e91c6ce24d165dab93e86433"},
		{"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", "512079be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"},
	}
	for _, c := range cases {
		hrp := "bc"
		if strings.HasPrefix(strings.ToLower(c.addr), "tb") {
			hrp = "tb"
		}
		version, program, err := DecodeBech32Address(c.addr, hrp)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.addr, err)
			continue
		}
		if got := segwitScriptHex(version, program); got != c.wantScript {
			t.Errorf("%s: script = %s, want %s", c.addr, got, c.wantScript)
		}
	}
}

// The invalid address vectors from BIP350. Each must be rejected, under both
// prefixes.
func TestDecodeBech32Address_BIPInvalidVectors(t *testing.T) {
	cases := []struct{ addr, why string }{
		{"tc1qw508d6qejxtdg4y5r3zarvary0c5xw7kg3g4ty", "invalid prefix"},
		{"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqh2y7hd", "version 1 with a Bech32 checksum"},
		{"tb1z0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqglt7rf", "version 2 with a Bech32 checksum"},
		{"BC1S0XLXVLHEMJA6C4DQV22UAPCTQUPFHLXM9H8Z3K2E72Q4K9HCZ7VQ54WELL", "version 16 with a Bech32 checksum"},
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kemeawh", "version 0 with a Bech32m checksum"},
		{"tb1q0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vq24jc47", "version 0 with a Bech32m checksum"},
		{"bc1p38j9r5y49hruaue7wxjce0updqjuyyx0kh56v8s25huc6995vvpql3jow4", "invalid character"},
		{"BC130XLXVLHEMJA6C4DQV22UAPCTQUPFHLXM9H8Z3K2E72Q4K9HCZ7VQ7ZWS8R", "witness version 17"},
		{"bc1pw5dgrnzv", "program too short (1 byte)"},
		{"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7v8n0nx0muaewav253zgeav", "program too long (41 bytes)"},
		{"BC1QR508D6QEJXTDG4Y5R3ZARVARYV98GJ9P", "version 0 program of 16 bytes"},
		{"tb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vq47Zagq", "mixed case"},
		{"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7v07qwwzcrf", "zero padding of more than 4 bits"},
		{"tb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vpggkg9", "non-zero padding"},
		{"bc1gmk9yu", "empty data"},
	}
	for _, c := range cases {
		for _, hrp := range []string{"bc", "tb"} {
			if _, _, err := DecodeBech32Address(c.addr, hrp); err == nil {
				t.Errorf("%s (%s) was accepted with prefix %q", c.addr, c.why, hrp)
			}
		}
	}
}

// Bech32Decode stays Bech32-only: a Bech32m string is rejected there.
func TestBech32Decode_RejectsBech32m(t *testing.T) {
	if _, _, err := Bech32Decode("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"); err != nil {
		t.Errorf("Bech32 string rejected: %v", err)
	}
	if _, _, err := Bech32Decode("bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0"); err == nil {
		t.Error("Bech32Decode accepted a Bech32m string")
	}
}

// testTaprootKey is the 32-byte output key used in BIP350's own vectors.
const testTaprootKey = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"

var testGenericBTCS = CoinDefinition{
	Name:   "Bitcoin Silver",
	Symbol: "BTCS",
	Segwit: true,
	Address: AddressConfig{
		Base58: &Base58Config{
			P2PKH: NetworkVersions{Mainnet: 26, Testnet: 111},
			P2SH:  &NetworkVersions{Mainnet: 5, Testnet: 196},
		},
		Bech32: &Bech32Config{HRP: NetworkHRP{Mainnet: "bs", Testnet: "tbs"}},
	},
}

// Taproot addresses pay to OP_1 PUSH32 <key> on every coin that uses Bech32.
// The DigiByte and BitcoinII/Bitcoin addresses were produced by the reference
// encoder from the BIP350 key; the Bitcoin Silver one is a real wallet address.
func TestAddressToScript_Taproot(t *testing.T) {
	btcs := NewGenericCoin("bitcoinsilver", testGenericBTCS)
	cases := []struct {
		name       string
		coin       Coin
		addr, net  string
		wantScript string
	}{
		{"Bitcoin mainnet", &Bitcoin{}, "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", "mainnet", "5120" + testTaprootKey},
		{"Bitcoin testnet", &Bitcoin{}, "tb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vq47zagq", "testnet", "5120" + testTaprootKey},
		{"BitcoinII mainnet", &BitcoinII{}, "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", "mainnet", "5120" + testTaprootKey},
		{"DigiByte mainnet", &DigiByte{}, "dgb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqlussds", "mainnet", "5120" + testTaprootKey},
		{"DigiByte testnet", &DigiByte{}, "dgbt1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vq5yalvm", "testnet", "5120" + testTaprootKey},
		{"generic coin (Bitcoin Silver), real address", btcs, "bs1pu4s7psh92uf47yg3e57dzq0l0g2xcrwjl2ak8dweyku0rpm2vacqyv4we8", "mainnet", "5120e561e0c2e557135f1111cd3cd101ff7a146c0dd2fabb63b5d925b8f1876a6770"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.coin.ValidateAddress(c.addr, c.net); err != nil {
				t.Errorf("ValidateAddress(%s): %v", c.addr, err)
			}
			script, err := c.coin.AddressToScript(c.addr, c.net)
			if err != nil {
				t.Fatalf("AddressToScript(%s): %v", c.addr, err)
			}
			if got := hex.EncodeToString(script); got != c.wantScript {
				t.Errorf("script = %s, want %s", got, c.wantScript)
			}
		})
	}
}

// SegWit v0 addresses are unchanged: same scripts as before Taproot support.
func TestAddressToScript_SegwitV0Unchanged(t *testing.T) {
	cases := []struct {
		name       string
		coin       Coin
		addr, net  string
		wantScript string
	}{
		{"Bitcoin P2WPKH", &Bitcoin{}, "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", "mainnet", "0014751e76e8199196d454941c45d1b3a323f1433bd6"},
		{"Bitcoin P2WSH testnet", &Bitcoin{}, "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", "testnet", "00201863143c14c5166804bd19203356da136c985678cd4d27a1b8c6329604903262"},
		{"DigiByte P2WPKH (the donation address)", &DigiByte{}, "dgb1q3ym6z2j87unymufd0g8m89xge3r2pr43a20h5l", "mainnet", "00148937a12a47f7264df12d7a0fb394c8cc46a08eb1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script, err := c.coin.AddressToScript(c.addr, c.net)
			if err != nil {
				t.Fatalf("AddressToScript: %v", err)
			}
			if got := hex.EncodeToString(script); got != c.wantScript {
				t.Errorf("script = %s, want %s", got, c.wantScript)
			}
		})
	}
}

// Addresses GSE can't pay are refused by BOTH ValidateAddress and
// AddressToScript, so nothing is accepted at start-up and then fails when a
// job is built.
func TestSegwit_UnpayableAddressesRejectedConsistently(t *testing.T) {
	btc := &Bitcoin{}
	cases := []struct{ addr, why string }{
		{"bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7kt5nd6y", "version 1 with a 40-byte program (not Taproot)"},
		{"BC1SW50QGDZ25J", "witness version 16"},
		{"bc1zw508d6qejxtdg4y5r3zarvaryvaxxpcs", "witness version 2"},
		{"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqh2y7hd", "Taproot key with a Bech32 checksum"},
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kemeawh", "version 0 with a Bech32m checksum"},
		{"dgb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqlussds", "another coin's prefix"},
	}
	for _, c := range cases {
		if err := btc.ValidateAddress(c.addr, "mainnet"); err == nil {
			t.Errorf("ValidateAddress accepted %s (%s)", c.addr, c.why)
		}
		if _, err := btc.AddressToScript(c.addr, "mainnet"); err == nil {
			t.Errorf("AddressToScript accepted %s (%s)", c.addr, c.why)
		}
	}
}

// A mainnet Taproot address is not valid on testnet, and the reverse.
func TestTaproot_WrongNetworkRejected(t *testing.T) {
	dgb := &DigiByte{}
	if _, err := dgb.AddressToScript("dgb1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqlussds", "testnet"); err == nil {
		t.Error("mainnet Taproot address accepted on testnet")
	}
	if _, err := dgb.AddressToScript("dgbt1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vq5yalvm", "mainnet"); err == nil {
		t.Error("testnet Taproot address accepted on mainnet")
	}
}

// A generic coin without SegWit never treats an address as Bech32.
func TestTaproot_GenericCoinWithoutSegwit(t *testing.T) {
	def := testGenericBTCS
	def.Segwit = false
	c := NewGenericCoin("bitcoinsilver", def)
	if _, err := c.AddressToScript("bs1pu4s7psh92uf47yg3e57dzq0l0g2xcrwjl2ak8dweyku0rpm2vacqyv4we8", "mainnet"); err == nil {
		t.Error("Taproot address accepted by a coin definition with segwit disabled")
	}
}
