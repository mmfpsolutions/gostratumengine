/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package stratum

import "encoding/json"

// Request represents a JSON-RPC request from a miner.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response represents a JSON-RPC response to a miner.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result"`
	Error   interface{}     `json:"error"`
}

// Notification represents a JSON-RPC notification to a miner (no ID).
type Notification struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      interface{}   `json:"id"`
}

// Job represents a mining job sent to miners via mining.notify.
type Job struct {
	JobID          string   // Unique job identifier
	PrevHash       string   // Previous block hash (word-swapped hex)
	Coinb1         string   // Coinbase part 1 (hex)
	Coinb2         string   // Coinbase part 2 (hex)
	MerkleBranches []string // Merkle branch hashes (hex)
	Version        string   // Block version (hex, little-endian)
	NBits          string   // Target bits (hex)
	NTime          string   // Block timestamp (hex)
	CleanJobs      bool     // Whether to discard old jobs
}

// ToNotifyParams converts a Job to the params array for mining.notify.
func (j *Job) ToNotifyParams() []interface{} {
	branches := make([]interface{}, len(j.MerkleBranches))
	for i, b := range j.MerkleBranches {
		branches[i] = b
	}
	return []interface{}{
		j.JobID,
		j.PrevHash,
		j.Coinb1,
		j.Coinb2,
		branches,
		j.Version,
		j.NBits,
		j.NTime,
		j.CleanJobs,
	}
}

// ShareSubmission represents a mining.submit from a miner.
type ShareSubmission struct {
	WorkerName  string
	JobID       string
	ExtraNonce2 string
	NTime       string
	Nonce       string
	VersionBits string // Version rolling bits (optional, from mining.configure)
}

// ParseShareSubmission extracts share data from mining.submit params.
func ParseShareSubmission(params json.RawMessage) (*ShareSubmission, error) {
	var raw []string
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil, err
	}

	if len(raw) < 5 {
		return nil, ErrMalformedRequest
	}

	share := &ShareSubmission{
		WorkerName:  raw[0],
		JobID:       raw[1],
		ExtraNonce2: raw[2],
		NTime:       raw[3],
		Nonce:       raw[4],
	}

	// Optional version bits (mining.configure version rolling)
	if len(raw) >= 6 {
		share.VersionBits = raw[5]
	}

	return share, nil
}

// SubscribeResult builds the result for mining.subscribe response.
func SubscribeResult(sessionID, extraNonce1 string, extraNonce2Size int) interface{} {
	return []interface{}{
		[][]string{
			{"mining.set_difficulty", sessionID},
			{"mining.notify", sessionID},
		},
		extraNonce1,
		extraNonce2Size,
	}
}

// SetDifficultyNotification creates a mining.set_difficulty notification.
func SetDifficultyNotification(difficulty float64) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		ID:      nil,
		Method:  "mining.set_difficulty",
		Params:  []interface{}{difficulty},
	}
}

// NotifyNotification creates a mining.notify notification from a Job.
func NotifyNotification(job *Job) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		ID:      nil,
		Method:  "mining.notify",
		Params:  job.ToNotifyParams(),
	}
}

// PingRequest creates a mining.ping request sent from server to miner.
func PingRequest(id interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "mining.ping",
		"params":  []interface{}{},
	}
}

// ConfigureResult builds the result for mining.configure response.
// Currently supports version-rolling extension.
type ConfigureResult struct {
	VersionRolling         bool   `json:"version-rolling,omitempty"`
	VersionRollingMask     string `json:"version-rolling.mask,omitempty"`
	VersionRollingMinBit   int    `json:"version-rolling.min-bit-count,omitempty"`
}

// Stratum error codes.
var (
	ErrOther           = stratumError{20, "Other/Unknown"}
	ErrJobNotFound     = stratumError{21, "Job not found"}
	ErrDuplicateShare  = stratumError{22, "Duplicate share"}
	ErrLowDifficulty   = stratumError{23, "Low difficulty share"}
	ErrUnauthorized    = stratumError{24, "Unauthorized worker"}
	ErrNotSubscribed   = stratumError{25, "Not subscribed"}
	ErrMalformedRequest = stratumError{26, "Malformed request"}
)

type stratumError struct {
	Code    int
	Message string
}

func (e stratumError) Error() string {
	return e.Message
}

// ToJSON converts a stratum error to the JSON-RPC error format.
func (e stratumError) ToJSON() interface{} {
	return []interface{}{e.Code, e.Message, nil}
}
