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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/coin"
	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/metrics"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
	"github.com/mmfpsolutions/gostratumengine/pkg/stratum"
)

// ecashBlockCooldown is the duration after submitting an eCash block
// during which new block submissions are suppressed to avoid submitting
// on Avalanche-parked chains.
const ecashBlockCooldown = 30 * time.Second

// ShareValidator validates share submissions from miners.
type ShareValidator struct {
	coin              coin.Coin
	jobMgr            *JobManager
	rpcClient         *noderpc.Client
	stats             *metrics.Stats
	symbol            string
	soloMode          bool
	staleShareGrace   time.Duration // grace period to accept shares after a new block
	lowDiffShareGrace time.Duration // grace period to accept shares at previous diff after a change
	logger            *logging.Logger
	lastBlockSubmit   time.Time // eCash: time of last block submission
}

// NewShareValidator creates a new share validator.
func NewShareValidator(c coin.Coin, jobMgr *JobManager, rpcClient *noderpc.Client, stats *metrics.Stats, soloMode bool, staleShareGrace, lowDiffShareGrace time.Duration) *ShareValidator {
	return &ShareValidator{
		coin:              c,
		jobMgr:            jobMgr,
		rpcClient:         rpcClient,
		stats:             stats,
		symbol:            c.Symbol(),
		soloMode:          soloMode,
		staleShareGrace:   staleShareGrace,
		lowDiffShareGrace: lowDiffShareGrace,
		logger:            logging.New(logging.ModuleEngine),
	}
}

// ValidateShare processes a share submission from a miner session.
// This follows the same approach as GSS: reconstruct coinbase from job data,
// compute merkle root using branches, build header, and check difficulty.
func (sv *ShareValidator) ValidateShare(session *stratum.Session, share *stratum.ShareSubmission) error {
	// Look up the job
	jobData := sv.jobMgr.GetJob(share.JobID)
	if jobData == nil {
		sv.stats.RecordShare(sv.symbol, metrics.ShareStale, share.WorkerName, 0)
		return stratum.ErrJobNotFound
	}

	// Check if job is stale (different previous block hash).
	// Allow a grace period after a new block so in-flight shares aren't rejected.
	currentTip := sv.jobMgr.CurrentTip()
	if jobData.Template.PreviousBlockHash != currentTip {
		if sv.staleShareGrace > 0 && time.Since(sv.jobMgr.TipChangedAt()) < sv.staleShareGrace {
			sv.logger.Debug("stale share accepted within grace period: worker=%s job=%s", share.WorkerName, share.JobID)
		} else {
			sv.stats.RecordShare(sv.symbol, metrics.ShareStale, share.WorkerName, 0)
			return stratum.ErrJobNotFound
		}
	}

	// Get the correct coinb2 for this session
	coinb2 := jobData.Coinb2
	if sv.soloMode {
		addr := session.MiningAddress()
		if addrCoinb2, ok := jobData.AddressCoinb2s[addr]; ok {
			coinb2 = addrCoinb2
		} else {
			sv.stats.RecordShare(sv.symbol, metrics.ShareStale, share.WorkerName, 0)
			return stratum.ErrJobNotFound
		}
	}

	// Reconstruct the coinbase transaction (in txid format — no witness data)
	fullCoinbaseHex := jobData.Coinb1 + session.ExtraNonce1() + share.ExtraNonce2 + coinb2
	coinbaseBytes, err := hex.DecodeString(fullCoinbaseHex)
	if err != nil {
		sv.stats.RecordShare(sv.symbol, metrics.ShareInvalid, share.WorkerName, 0)
		return stratum.ErrMalformedRequest
	}

	// Coinbase hash — simple double SHA256 (no witness stripping needed,
	// because coinb1/coinb2 are already in txid format)
	coinbaseHash := coinbase.DoubleSHA256(coinbaseBytes)

	// Build merkle root using branches from the job (same as what the miner does)
	merkleRoot := make([]byte, 32)
	copy(merkleRoot, coinbaseHash)
	for _, branchHex := range jobData.Job.MerkleBranches {
		branchBytes, err := hex.DecodeString(branchHex)
		if err != nil {
			continue
		}
		combined := make([]byte, 64)
		copy(combined[:32], merkleRoot)
		copy(combined[32:], branchBytes)
		merkleRoot = coinbase.DoubleSHA256(combined)
	}

	// Build 80-byte block header (matching GSS's ConstructHeader approach)
	header := make([]byte, 0, 80)

	// Version (4 bytes) — XOR base version with version bits (like GSS)
	baseVersionBytes, err := hex.DecodeString(jobData.Job.Version)
	if err != nil || len(baseVersionBytes) != 4 {
		return fmt.Errorf("invalid job version: %s", jobData.Job.Version)
	}
	baseVersion := binary.BigEndian.Uint32(baseVersionBytes)
	finalVersion := baseVersion
	if share.VersionBits != "" {
		vbitsBytes, err := hex.DecodeString(share.VersionBits)
		if err == nil && len(vbitsBytes) == 4 {
			rolledBits := binary.BigEndian.Uint32(vbitsBytes)
			finalVersion = baseVersion ^ rolledBits
		}
	}
	versionLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(versionLE, finalVersion)
	header = append(header, versionLE...)

	// Previous block hash (32 bytes)
	// job.PrevHash is in stratum format (word-swapped from display).
	// swapEndianWords un-swaps it back to internal byte order for the header.
	prevHashBytes := swapEndianWords(jobData.Job.PrevHash)
	header = append(header, prevHashBytes...)

	// Merkle root (32 bytes)
	header = append(header, merkleRoot...)

	// Timestamp (4 bytes) — parse as big-endian hex, write as little-endian
	nTimeBytes, err := hex.DecodeString(share.NTime)
	if err != nil || len(nTimeBytes) != 4 {
		sv.stats.RecordShare(sv.symbol, metrics.ShareInvalid, share.WorkerName, 0)
		return stratum.ErrMalformedRequest
	}
	nTimeUint := binary.BigEndian.Uint32(nTimeBytes)
	nTimeLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(nTimeLE, nTimeUint)
	header = append(header, nTimeLE...)

	// Bits (4 bytes) — parse as big-endian hex, write as little-endian
	bitsBytes, err := hex.DecodeString(jobData.Job.NBits)
	if err != nil || len(bitsBytes) != 4 {
		return fmt.Errorf("invalid nbits: %s", jobData.Job.NBits)
	}
	bitsUint := binary.BigEndian.Uint32(bitsBytes)
	bitsLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(bitsLE, bitsUint)
	header = append(header, bitsLE...)

	// Nonce (4 bytes) — parse as big-endian hex, write as little-endian
	nonceBytes, err := hex.DecodeString(share.Nonce)
	if err != nil || len(nonceBytes) != 4 {
		sv.stats.RecordShare(sv.symbol, metrics.ShareInvalid, share.WorkerName, 0)
		return stratum.ErrMalformedRequest
	}
	nonceUint := binary.BigEndian.Uint32(nonceBytes)
	nonceLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(nonceLE, nonceUint)
	header = append(header, nonceLE...)

	// Double SHA256 the header
	blockHash := coinbase.DoubleSHA256(header)

	// Reverse for big-endian display / comparison
	blockHashBE := coinbase.ReverseBytes(blockHash)
	blockHashHex := hex.EncodeToString(blockHashBE)

	// Calculate actual share difficulty
	hashInt := new(big.Int).SetBytes(blockHashBE)
	maxTarget := new(big.Int)
	maxTarget.SetString("00000000FFFF0000000000000000000000000000000000000000000000000000", 16)
	actualDiff := float64(0)
	if hashInt.Sign() > 0 {
		actualDiffBig := new(big.Float).Quo(new(big.Float).SetInt(maxTarget), new(big.Float).SetInt(hashInt))
		actualDiff, _ = actualDiffBig.Float64()
	}

	// Check pool difficulty
	sessionDiff := session.GetDifficulty()
	if !coin.HashMeetsDifficulty(blockHashBE, sessionDiff) {
		// Grace period: accept shares at the previous difficulty for a short window
		// after a difficulty change (covers suggest_difficulty, vardiff, password diff)
		accepted := false
		if sv.lowDiffShareGrace > 0 {
			prevDiff, changedAt := session.GetPrevDifficulty()
			if prevDiff > 0 && time.Since(changedAt) < sv.lowDiffShareGrace {
				if coin.HashMeetsDifficulty(blockHashBE, prevDiff) {
					sv.logger.Debug("low-diff share accepted within grace period: worker=%s diff=%g prevDiff=%g currentDiff=%g",
						share.WorkerName, actualDiff, prevDiff, sessionDiff)
					accepted = true
				}
			}
		}
		if !accepted {
			sv.logger.Debug("share rejected: worker=%s required=%g actual=%g hash=%s", share.WorkerName, sessionDiff, actualDiff, blockHashHex)
			sv.stats.RecordShare(sv.symbol, metrics.ShareInvalid, share.WorkerName, actualDiff)
			return stratum.ErrLowDifficulty
		}
	}

	// Share is valid at pool difficulty
	sv.stats.RecordShare(sv.symbol, metrics.ShareValid, share.WorkerName, actualDiff)
	sv.logger.Debug("share accepted: worker=%s diff=%g target=%g hash=%s", share.WorkerName, actualDiff, sessionDiff, blockHashHex)

	// Check if it meets the network target (potential block!)
	nbits, _ := coin.BitsToHex(jobData.Job.NBits)
	targetBig := coin.CompactToBig(nbits)
	if coin.HashMeetsTarget(blockHashBE, targetBig) {
		sv.logger.Info("*** POTENTIAL BLOCK FOUND by %s at height %d! *** hash=%s", share.WorkerName, jobData.Template.Height, blockHashHex)

		// eCash block cooldown: suppress submissions shortly after a block
		// to avoid submitting on Avalanche-parked chains
		if sv.symbol == "XEC" && !sv.lastBlockSubmit.IsZero() {
			elapsed := time.Since(sv.lastBlockSubmit)
			if elapsed < ecashBlockCooldown {
				sv.logger.Warn("eCash block cooldown active (%.0fs remaining) - skipping submission",
					(ecashBlockCooldown - elapsed).Seconds())
				return nil
			}
		}

		// eCash chain reorg check: verify the chain tip hasn't changed
		// between when the template was fetched and now
		if sv.symbol == "XEC" {
			bestHash, err := sv.rpcClient.GetBestBlockHash()
			if err != nil {
				sv.logger.Warn("could not verify chain tip before submission: %v", err)
			} else if bestHash != jobData.Template.PreviousBlockHash {
				sv.logger.Warn("eCash chain reorg detected: template prevhash=%s, current tip=%s - skipping submission",
					jobData.Template.PreviousBlockHash, bestHash)
				return nil
			}
		}

		// RTT (Real Time Target) validation for eCash only
		if sv.symbol == "XEC" && jobData.Template.RTT != nil {
			now := time.Now().Unix()
			if !coin.IsRTTDataValid(jobData.Template) {
				sv.logger.Warn("RTT data malformed (all timestamps identical) - letting node decide on block validity")
			} else {
				rttValid, rttErr := coin.CheckRTTTarget(blockHashBE, jobData.Template, now)
				if rttErr != nil {
					sv.logger.Error("RTT computation error: %v - attempting submission anyway", rttErr)
				} else if !rttValid {
					sv.logger.Warn("block rejected by RTT - hash does not meet Real Time Target, not submitting")
					return nil
				} else {
					sv.logger.Info("RTT check passed - block meets Real Time Target")
				}
			}
		}

		// Build and submit the block
		blockHex, err := sv.coin.BuildBlock(header, coinbaseBytes, jobData.Template)
		if err != nil {
			sv.logger.Error("building block: %v", err)
			return nil // share is still valid
		}

		if err := sv.rpcClient.SubmitBlock(blockHex); err != nil {
			sv.logger.Error("submitting block: %v", err)
			return nil
		}

		// Record submission time for eCash cooldown
		if sv.symbol == "XEC" {
			sv.lastBlockSubmit = time.Now()
		}

		// Verify submission
		actualHash, err := sv.rpcClient.GetBlockHash(jobData.Template.Height)
		if err != nil {
			sv.logger.Warn("could not verify block submission: %v", err)
		} else {
			if actualHash == blockHashHex {
				sv.logger.Info("block %s confirmed at height %d!", blockHashHex, jobData.Template.Height)
				sv.stats.RecordBlock(sv.symbol, blockHashHex, jobData.Template.Height, share.WorkerName)
			} else {
				sv.logger.Warn("block not confirmed: expected %s, got %s", blockHashHex, actualHash)
			}
		}
	}

	return nil
}

// swapEndianWords performs word-level (4-byte) endian swapping on a hex string.
// This converts the stratum prevhash format back to internal byte order for the block header.
func swapEndianWords(hexStr string) []byte {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil
	}

	result := make([]byte, len(data))
	for i := 0; i < len(data); i += 4 {
		if i+4 <= len(data) {
			result[i] = data[i+3]
			result[i+1] = data[i+2]
			result[i+2] = data[i+1]
			result[i+3] = data[i]
		}
	}
	return result
}
