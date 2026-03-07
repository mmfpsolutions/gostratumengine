/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package noderpc

import "encoding/json"

// rpcRequest is a JSON-RPC 1.0 request.
type rpcRequest struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// rpcResponse is a JSON-RPC 1.0 response.
type rpcResponse struct {
	ID     interface{}      `json:"id"`
	Result json.RawMessage  `json:"result"`
	Error  *rpcError        `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return e.Message
}

// BlockTemplate is the response from getblocktemplate.
type BlockTemplate struct {
	Version               uint32                 `json:"version"`
	PreviousBlockHash     string                 `json:"previousblockhash"`
	Transactions          []BlockTemplateTransaction `json:"transactions"`
	CoinbaseAux           map[string]string      `json:"coinbaseaux"`
	CoinbaseValue         int64                  `json:"coinbasevalue"`
	Target                string                 `json:"target"`
	MinTime               int64                  `json:"mintime"`
	Mutable               []string               `json:"mutable"`
	NonceRange            string                 `json:"noncerange"`
	SigOpLimit            int                    `json:"sigoplimit"`
	SizeLimit             int                    `json:"sizelimit"`
	WeightLimit           int                    `json:"weightlimit"`
	CurTime               int64                  `json:"curtime"`
	Bits                  string                 `json:"bits"`
	Height                int64                  `json:"height"`
	DefaultWitnessCommitment string              `json:"default_witness_commitment"`
	Rules                 []string               `json:"rules"`
	// eCash-specific fields
	CoinbaseTxn           *CoinbaseTxn           `json:"coinbasetxn,omitempty"`
	RTT                   *RTTData               `json:"rtt,omitempty"`
}

// BlockTemplateTransaction represents a transaction in a block template.
type BlockTemplateTransaction struct {
	Data    string `json:"data"`
	TxID    string `json:"txid"`
	Hash    string `json:"hash"`
	Fee     int64  `json:"fee"`
	SigOps  int    `json:"sigops"`
	Weight  int    `json:"weight"`
}

// CoinbaseTxn represents eCash-specific coinbase transaction requirements.
type CoinbaseTxn struct {
	MinerFund      *MinerFund      `json:"minerfund,omitempty"`
	StakingRewards *StakingRewards `json:"stakingrewards,omitempty"`
}

// MinerFund represents eCash miner fund requirements.
type MinerFund struct {
	Addresses    []string `json:"addresses"`
	MinimumValue int64    `json:"minimumvalue"`
}

// StakingRewardsScript represents the payout script for staking rewards.
type StakingRewardsScript struct {
	Hex string `json:"hex"`
}

// StakingRewards represents eCash staking reward requirements.
type StakingRewards struct {
	PayoutScript StakingRewardsScript `json:"payoutscript"`
	MinimumValue int64                `json:"minimumvalue"`
}

// RTTData represents Real Time Targeting data for eCash.
type RTTData struct {
	PrevHeaderTime []int64 `json:"prevheadertime"`
	PrevBits       string  `json:"prevbits"`
	NodeTime       int64   `json:"nodetime"`
	NextTarget     string  `json:"nexttarget"`
}

// BlockchainInfo is the response from getblockchaininfo.
type BlockchainInfo struct {
	Chain         string  `json:"chain"`
	Blocks        int64   `json:"blocks"`
	Headers       int64   `json:"headers"`
	BestBlockHash string  `json:"bestblockhash"`
	Difficulty    float64 `json:"difficulty"`
}
