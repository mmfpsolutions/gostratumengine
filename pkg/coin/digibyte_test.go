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
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
)

// DigiDollar oracle commitment tests. A block that carries DigiDollar
// mint/redeem transactions is only valid if the coinbase carries the node's
// oracle commitment, byte for byte. These tests parse the coinbase GSE builds
// and compare every output.

const (
	// A real default_oracle_commitment from a DigiByte node (96 bytes), and a
	// real default_witness_commitment.
	testOracleCommitment  = "6abf01034c5a054e08002400d4080000d80900000000000048d64b6a00000000feed102a0177cc03d3672f8ec9102667436f7eed70b53c61349c3ac9ee3e8da6e2217e9c234ce81b8a8ce78da33c019eaef77627ca223213557865ddb09e27dc"
	testWitnessCommitment = "6a24aa21a9ede2f61c3f71d1defd3fa999dfa36953755c690689799962b48bebd836974e8cf9"

	testDGBPayoutAddress = "dgb1q3ym6z2j87unymufd0g8m89xge3r2pr43a20h5l"
	testDGBPayoutScript  = "00148937a12a47f7264df12d7a0fb394c8cc46a08eb1"
	testDonationScript   = "76a9142270f6143fd519a1e16f7cce9b51507b0f33f45488ac"
)

type parsedOutput struct {
	value  int64
	script string // hex
}

// parseCoinb2 parses coinb2: sequence(4) + output count + outputs + locktime(4).
func parseCoinb2(t *testing.T, coinb2Hex string) []parsedOutput {
	t.Helper()
	b, err := hex.DecodeString(coinb2Hex)
	if err != nil {
		t.Fatalf("decode coinb2: %v", err)
	}
	if len(b) < 9 {
		t.Fatalf("coinb2 too short: %d bytes", len(b))
	}
	if hex.EncodeToString(b[:4]) != "ffffffff" {
		t.Fatalf("coinb2 should start with the input sequence, got %x", b[:4])
	}
	count := int(b[4]) // tests use fewer than 253 outputs
	pos := 5
	outs := make([]parsedOutput, 0, count)
	for i := 0; i < count; i++ {
		if pos+9 > len(b) {
			t.Fatalf("coinb2 truncated at output %d", i)
		}
		value := int64(binary.LittleEndian.Uint64(b[pos : pos+8]))
		pos += 8
		n := int(b[pos]) // tests use scripts shorter than 253 bytes
		pos++
		if pos+n > len(b) {
			t.Fatalf("coinb2 truncated in the script of output %d", i)
		}
		outs = append(outs, parsedOutput{value, hex.EncodeToString(b[pos : pos+n])})
		pos += n
	}
	if rest := b[pos:]; hex.EncodeToString(rest) != "00000000" {
		t.Fatalf("coinb2 should end with a zero locktime, got %x", rest)
	}
	return outs
}

func dgbTemplate(oracle, witness string) *noderpc.BlockTemplate {
	return &noderpc.BlockTemplate{
		Height:                   24346255,
		CoinbaseValue:            25072839494,
		DefaultOracleCommitment:  oracle,
		DefaultWitnessCommitment: witness,
	}
}

func buildDGB(t *testing.T, tmpl *noderpc.BlockTemplate, extra []coinbase.CoinbaseOutput) (coinb1, coinb2 string) {
	t.Helper()
	coinb1, coinb2, err := (&DigiByte{}).BuildCoinbase(tmpl, testDGBPayoutAddress, "mainnet", "/DGB GSE/", 4, 4, extra)
	if err != nil {
		t.Fatalf("BuildCoinbase: %v", err)
	}
	return coinb1, coinb2
}

func donationOutput(t *testing.T, value int64) []coinbase.CoinbaseOutput {
	t.Helper()
	script, err := hex.DecodeString(testDonationScript)
	if err != nil {
		t.Fatal(err)
	}
	return []coinbase.CoinbaseOutput{{Value: value, Script: script}}
}

// The DigiByte template request opts in to DigiDollar-aware templates.
func TestDigiByte_TemplateRules(t *testing.T) {
	got := strings.Join((&DigiByte{}).TemplateRules(), ",")
	if got != "segwit,digidollar-oracle" {
		t.Errorf("TemplateRules = %q, want segwit,digidollar-oracle", got)
	}
}

// The node's field name decodes into the template.
func TestBlockTemplate_DecodesOracleCommitment(t *testing.T) {
	raw := `{"height":1,"default_witness_commitment":"` + testWitnessCommitment + `","default_oracle_commitment":"` + testOracleCommitment + `"}`
	var tmpl noderpc.BlockTemplate
	if err := jsonUnmarshal(raw, &tmpl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tmpl.DefaultOracleCommitment != testOracleCommitment {
		t.Errorf("DefaultOracleCommitment = %q", tmpl.DefaultOracleCommitment)
	}
}

// Oracle + witness, with a donation: payout, donation, ORACLE, WITNESS, in
// that order. Both commitments are zero-value and byte-identical to the
// template's. No value is lost.
func TestDigiByte_BuildCoinbase_OracleCommitment(t *testing.T) {
	tmpl := dgbTemplate(testOracleCommitment, testWitnessCommitment)
	const donation = 250728394
	_, coinb2 := buildDGB(t, tmpl, donationOutput(t, donation))
	outs := parseCoinb2(t, coinb2)

	want := []parsedOutput{
		{tmpl.CoinbaseValue - donation, testDGBPayoutScript},
		{donation, testDonationScript},
		{0, testOracleCommitment},
		{0, testWitnessCommitment},
	}
	if len(outs) != len(want) {
		t.Fatalf("outputs = %d, want %d: %+v", len(outs), len(want), outs)
	}
	for i := range want {
		if outs[i] != want[i] {
			t.Errorf("output %d = %+v, want %+v", i, outs[i], want[i])
		}
	}
	if last := outs[len(outs)-1].script; last != testWitnessCommitment {
		t.Errorf("the witness commitment must be the last output, got %s", last)
	}
}

// Without a donation: payout, oracle, witness.
func TestDigiByte_BuildCoinbase_OracleNoDonation(t *testing.T) {
	tmpl := dgbTemplate(testOracleCommitment, testWitnessCommitment)
	_, coinb2 := buildDGB(t, tmpl, nil)
	outs := parseCoinb2(t, coinb2)
	want := []parsedOutput{
		{tmpl.CoinbaseValue, testDGBPayoutScript},
		{0, testOracleCommitment},
		{0, testWitnessCommitment},
	}
	if len(outs) != len(want) {
		t.Fatalf("outputs = %d, want %d: %+v", len(outs), len(want), outs)
	}
	for i := range want {
		if outs[i] != want[i] {
			t.Errorf("output %d = %+v, want %+v", i, outs[i], want[i])
		}
	}
}

// No oracle commitment in the template (older node, DigiDollar inactive, or
// no bundle ready): nothing is added or invented. The coinbase is exactly
// payout, donation, witness, as before this feature.
func TestDigiByte_BuildCoinbase_NoOracleCommitment(t *testing.T) {
	tmpl := dgbTemplate("", testWitnessCommitment)
	const donation = 250728394
	coinb1, coinb2 := buildDGB(t, tmpl, donationOutput(t, donation))
	outs := parseCoinb2(t, coinb2)
	want := []parsedOutput{
		{tmpl.CoinbaseValue - donation, testDGBPayoutScript},
		{donation, testDonationScript},
		{0, testWitnessCommitment},
	}
	if len(outs) != len(want) {
		t.Fatalf("outputs = %d, want %d: %+v", len(outs), len(want), outs)
	}
	for i := range want {
		if outs[i] != want[i] {
			t.Errorf("output %d = %+v, want %+v", i, outs[i], want[i])
		}
	}

	// Known answer for the whole coinb2, assembled by hand from the transaction
	// format (sequence, 3 outputs, locktime; amounts little-endian). It equals
	// what GSE 1.0.5, before this feature, produced for the same inputs.
	wantCoinb2 := "ffffffff03" +
		"7c5b83c705000000" + "16" + testDGBPayoutScript +
		"cacff10e00000000" + "19" + testDonationScript +
		"0000000000000000" + "26" + testWitnessCommitment +
		"00000000"
	if coinb2 != wantCoinb2 {
		t.Errorf("coinb2 without an oracle commitment changed:\n got %s\nwant %s", coinb2, wantCoinb2)
	}

	// The oracle commitment only touches coinb2: coinb1 is the same either way.
	coinb1WithOracle, _ := buildDGB(t, dgbTemplate(testOracleCommitment, testWitnessCommitment), donationOutput(t, donation))
	if coinb1 != coinb1WithOracle {
		t.Error("coinb1 differs with an oracle commitment; only the outputs should change")
	}
}

// A commitment that isn't valid hex is an error. It must never be dropped
// silently: the template may contain transactions that need it.
func TestDigiByte_BuildCoinbase_InvalidOracleCommitment(t *testing.T) {
	_, _, err := (&DigiByte{}).BuildCoinbase(dgbTemplate("not-hex", testWitnessCommitment),
		testDGBPayoutAddress, "mainnet", "", 4, 4, nil)
	if err == nil {
		t.Fatal("BuildCoinbase accepted an oracle commitment that isn't hex")
	}
	if !strings.Contains(err.Error(), "oracle commitment") {
		t.Errorf("error %q should name the oracle commitment", err)
	}
}

// The commitment is copied, not rebuilt: whatever script the node returns
// is the script in the output.
func TestDigiByte_BuildCoinbase_OracleCopiedVerbatim(t *testing.T) {
	odd := "6a0401020304" // a short, different OP_RETURN
	_, coinb2 := buildDGB(t, dgbTemplate(odd, testWitnessCommitment), nil)
	outs := parseCoinb2(t, coinb2)
	if len(outs) != 3 || outs[1].script != odd || outs[1].value != 0 {
		t.Errorf("outputs = %+v, want the node's script %s copied as a zero-value output", outs, odd)
	}
}

// BuildBlock: the full coinbase in the block still has every output, with
// the SegWit marker/flag and witness added around them.
func TestDigiByte_BuildBlock_KeepsOracleOutput(t *testing.T) {
	tmpl := dgbTemplate(testOracleCommitment, testWitnessCommitment)
	coinb1, coinb2 := buildDGB(t, tmpl, nil)
	cb, err := coinbase.AssembleCoinbase(coinb1, "00000001", "00000002", coinb2)
	if err != nil {
		t.Fatalf("AssembleCoinbase: %v", err)
	}
	header := make([]byte, 80)
	blockHex, err := (&DigiByte{}).BuildBlock(header, cb, tmpl)
	if err != nil {
		t.Fatalf("BuildBlock: %v", err)
	}
	// header + tx count (1) + coinbase with marker/flag
	body := blockHex[160:]
	if !strings.HasPrefix(body, "01"+"01000000"+"0001") {
		t.Errorf("block should hold 1 transaction starting version + SegWit marker/flag, got %s…", body[:20])
	}
	oracleAt := strings.Index(body, testOracleCommitment)
	witnessAt := strings.Index(body, testWitnessCommitment)
	if oracleAt < 0 || witnessAt < 0 {
		t.Fatalf("block coinbase is missing a commitment (oracle at %d, witness at %d)", oracleAt, witnessAt)
	}
	if oracleAt > witnessAt {
		t.Error("oracle commitment must come before the witness commitment")
	}
	// Ends: witness stack (1 item, 32 zero bytes) + locktime.
	if !strings.HasSuffix(body, "0120"+strings.Repeat("00", 32)+"00000000") {
		t.Error("block coinbase should end with the witness reserved value and locktime")
	}
}

// DigiByte names its algorithm in every template request, so the template
// never depends on the node's "algo=" setting. A node set to another
// algorithm (odo, scrypt) otherwise returns that algorithm's target, every
// share looks like a block, and every submission fails with "high-hash".
func TestDigiByte_TemplateNamesTheAlgorithm(t *testing.T) {
	params := TemplateExtraParams(&DigiByte{})
	if len(params) != 1 || params[0] != "sha256d" {
		t.Errorf("DigiByte template params = %v, want [sha256d]", params)
	}
}

// Single-algorithm coins send nothing extra: their request is unchanged.
func TestTemplateExtraParams_OtherCoinsSendNothing(t *testing.T) {
	others := map[string]Coin{
		"bitcoin":     &Bitcoin{},
		"bitcoinii":   &BitcoinII{},
		"bitcoincash": &BitcoinCash{},
		"ecash":       &ECash{},
		"generic":     NewGenericCoin("bitcoinsilver", testGenericBTCS),
	}
	for name, c := range others {
		if params := TemplateExtraParams(c); len(params) != 0 {
			t.Errorf("%s sends extra template params %v, want none", name, params)
		}
	}
}
