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
	"sync"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/coin"
	"github.com/mmfpsolutions/gostratumengine/pkg/coinbase"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
	"github.com/mmfpsolutions/gostratumengine/pkg/stratum"
)

// JobData holds a job and its associated block template.
type JobData struct {
	Job            *stratum.Job
	Template       *noderpc.BlockTemplate
	Coinb1         string
	Coinb2         string
	AddressCoinb2s map[string]string // solo mode: address -> coinb2
	Created        time.Time
}

// JobManager manages block template polling and mining job creation.
type JobManager struct {
	coin           coin.Coin
	rpcClient      *noderpc.Client
	address        string
	network        string
	coinbaseText   string
	extraNonce1Size int
	extraNonce2Size int
	pollInterval   time.Duration

	jobs           map[string]*JobData
	currentTip     string
	jobCounter     uint64
	maxJobHistory  int
	mu             sync.RWMutex

	// Solo mode
	soloMode       bool
	connectedAddrs map[string]int // address -> session count
	addrMu         sync.RWMutex

	onNewJob func(*stratum.Job)
	logger   *logging.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// JobManagerConfig holds configuration for the job manager.
type JobManagerConfig struct {
	Coin            coin.Coin
	RPCClient       *noderpc.Client
	Address         string
	Network         string
	CoinbaseText    string
	ExtraNonceSize  int
	PollInterval    time.Duration
	OnNewJob        func(*stratum.Job)
	SoloMode        bool
}

// NewJobManager creates a new job manager.
func NewJobManager(cfg JobManagerConfig) *JobManager {
	// Split extranonce: first 4 bytes for extranonce1, rest for extranonce2
	en1Size := 4
	en2Size := cfg.ExtraNonceSize - en1Size
	if en2Size < 2 {
		en2Size = 2
	}

	jm := &JobManager{
		coin:            cfg.Coin,
		rpcClient:       cfg.RPCClient,
		address:         cfg.Address,
		network:         cfg.Network,
		coinbaseText:    cfg.CoinbaseText,
		extraNonce1Size: en1Size,
		extraNonce2Size: en2Size,
		pollInterval:    cfg.PollInterval,
		jobs:            make(map[string]*JobData),
		maxJobHistory:   10,
		soloMode:        cfg.SoloMode,
		onNewJob:        cfg.OnNewJob,
		logger:          logging.New(logging.ModuleEngine),
		stopCh:          make(chan struct{}),
	}
	if cfg.SoloMode {
		jm.connectedAddrs = make(map[string]int)
	}
	return jm
}

// Start begins polling for new block templates.
func (jm *JobManager) Start() error {
	// Fetch initial template
	if err := jm.refreshTemplate(true); err != nil {
		return fmt.Errorf("fetching initial template: %w", err)
	}

	jm.wg.Add(1)
	go jm.pollLoop()

	return nil
}

// Stop halts template polling.
func (jm *JobManager) Stop() {
	close(jm.stopCh)
	jm.wg.Wait()
}

// OnBlockNotification should be called when ZMQ sends a hashblock event.
// Forces an immediate template refresh.
func (jm *JobManager) OnBlockNotification(blockHash string) {
	jm.logger.Info("ZMQ block notification: %s", blockHash)
	if err := jm.refreshTemplate(true); err != nil {
		jm.logger.Error("template refresh after ZMQ notification: %v", err)
	}
}

// GetJob returns job data by job ID.
func (jm *JobManager) GetJob(jobID string) *JobData {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	return jm.jobs[jobID]
}

// CurrentTip returns the current best block hash.
func (jm *JobManager) CurrentTip() string {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	return jm.currentTip
}

// ExtraNonce1Size returns the extranonce1 byte size.
func (jm *JobManager) ExtraNonce1Size() int {
	return jm.extraNonce1Size
}

// ExtraNonce2Size returns the extranonce2 byte size.
func (jm *JobManager) ExtraNonce2Size() int {
	return jm.extraNonce2Size
}

func (jm *JobManager) pollLoop() {
	defer jm.wg.Done()
	ticker := time.NewTicker(jm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-jm.stopCh:
			return
		case <-ticker.C:
			if err := jm.refreshTemplate(false); err != nil {
				jm.logger.Error("template refresh: %v", err)
			}
		}
	}
}

func (jm *JobManager) refreshTemplate(force bool) error {
	jm.logger.Debug("polling node for new template (force=%v)", force)

	template, err := jm.rpcClient.GetBlockTemplate(jm.coin.TemplateRules())
	if err != nil {
		return err
	}

	jm.logger.Debug("template received: height=%d, prevhash=%s, txns=%d",
		template.Height, template.PreviousBlockHash[:16]+"...", len(template.Transactions))

	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Determine if this is a new block
	cleanJobs := template.PreviousBlockHash != jm.currentTip
	if !cleanJobs && !force {
		// Same block, no forced refresh — skip unless transactions changed
		// For simplicity, always create a new job on poll to capture new transactions
	}

	jm.currentTip = template.PreviousBlockHash

	// Build coinbase
	var coinb1, coinb2 string
	var addressCoinb2s map[string]string

	if jm.soloMode {
		// Solo mode: build coinb2 per connected address
		jm.addrMu.RLock()
		addresses := make([]string, 0, len(jm.connectedAddrs))
		for addr := range jm.connectedAddrs {
			addresses = append(addresses, addr)
		}
		jm.addrMu.RUnlock()

		addressCoinb2s = make(map[string]string, len(addresses))
		for i, addr := range addresses {
			c1, c2, err := jm.coin.BuildCoinbase(
				template, addr, jm.network, jm.coinbaseText,
				jm.extraNonce1Size, jm.extraNonce2Size,
			)
			if err != nil {
				jm.logger.Error("building coinbase for address %s: %v", addr, err)
				continue
			}
			if i == 0 {
				coinb1 = c1 // coinb1 is the same for all addresses
			}
			addressCoinb2s[addr] = c2
		}
		// If no addresses connected, compute coinb1 with fallback address
		if coinb1 == "" && jm.address != "" {
			coinb1, _, _ = jm.coin.BuildCoinbase(
				template, jm.address, jm.network, jm.coinbaseText,
				jm.extraNonce1Size, jm.extraNonce2Size,
			)
		}
	} else {
		// Pool mode: single coinbase for all miners
		var err error
		coinb1, coinb2, err = jm.coin.BuildCoinbase(
			template, jm.address, jm.network, jm.coinbaseText,
			jm.extraNonce1Size, jm.extraNonce2Size,
		)
		if err != nil {
			return fmt.Errorf("building coinbase: %w", err)
		}
	}

	// Compute merkle branches from template transactions
	branches := jm.computeMerkleBranches(template)

	// Generate job ID
	jm.jobCounter++
	jobID := fmt.Sprintf("%016x", jm.jobCounter)

	// Encode block version as little-endian hex
	version := fmt.Sprintf("%08x", template.Version)

	// Swap previous block hash to word-reversed form for stratum
	prevHash := swapHashEndianness(template.PreviousBlockHash)

	job := &stratum.Job{
		JobID:          jobID,
		PrevHash:       prevHash,
		Coinb1:         coinb1,
		Coinb2:         coinb2,
		MerkleBranches: branches,
		Version:        version,
		NBits:          template.Bits,
		NTime:          fmt.Sprintf("%08x", template.CurTime),
		CleanJobs:      cleanJobs,
	}

	jobData := &JobData{
		Job:            job,
		Template:       template,
		Coinb1:         coinb1,
		Coinb2:         coinb2,
		AddressCoinb2s: addressCoinb2s,
		Created:        time.Now(),
	}

	jm.jobs[jobID] = jobData

	// Prune old jobs
	if len(jm.jobs) > jm.maxJobHistory {
		jm.pruneJobs()
	}

	if cleanJobs {
		jm.logger.Info("new block detected at height %d", template.Height)
	}

	// Notify (unlock first to avoid holding lock during broadcast)
	jm.mu.Unlock()
	if jm.onNewJob != nil {
		jm.onNewJob(job)
	}
	jm.mu.Lock()

	return nil
}

func (jm *JobManager) computeMerkleBranches(template *noderpc.BlockTemplate) []string {
	if len(template.Transactions) == 0 {
		return []string{}
	}

	// Collect transaction hashes (these are the "sibling" hashes for merkle branch)
	var txHashes [][]byte
	for _, tx := range template.Transactions {
		// Use the txid (or hash for segwit) as bytes
		hashHex := tx.TxID
		if hashHex == "" {
			hashHex = tx.Hash
		}
		hashBytes, err := hex.DecodeString(hashHex)
		if err != nil {
			continue
		}
		// txid is displayed in big-endian, but merkle tree uses little-endian (internal byte order)
		txHashes = append(txHashes, coinbase.ReverseBytes(hashBytes))
	}

	// For stratum, we send the merkle branch (sibling hashes at each level)
	// The miner computes: hash = SHA256d(coinbase_hash + branch[0]), then hash = SHA256d(hash + branch[1]), etc.
	branches := computeStratumMerkleBranches(txHashes)

	// Convert to hex strings
	hexBranches := make([]string, len(branches))
	for i, b := range branches {
		hexBranches[i] = hex.EncodeToString(b)
	}

	return hexBranches
}

// computeStratumMerkleBranches computes the sibling hashes needed by the miner
// to compute the merkle root from the coinbase hash.
// Given transaction hashes [t1, t2, t3, ...] (NOT including coinbase),
// this builds the tree with a placeholder at index 0 for the coinbase,
// and at each level records the sibling at index 1 (the branch hash).
// This matches the GSS implementation.
func computeStratumMerkleBranches(txHashes [][]byte) [][]byte {
	if len(txHashes) == 0 {
		return nil
	}

	// Build current level: [placeholder(coinbase), t1, t2, t3, ...]
	// The placeholder at index 0 represents the coinbase hash position.
	currentLevel := make([][]byte, len(txHashes)+1)
	currentLevel[0] = make([]byte, 32) // placeholder for coinbase
	copy(currentLevel[1:], txHashes)

	var result [][]byte

	for len(currentLevel) > 1 {
		// The sibling of our running hash is at index 1
		branch := make([]byte, len(currentLevel[1]))
		copy(branch, currentLevel[1])
		result = append(result, branch)

		// Compute next level by pairing
		nextLen := (len(currentLevel) + 1) / 2
		nextLevel := make([][]byte, nextLen)
		for i := 0; i < nextLen; i++ {
			leftIdx := i * 2
			rightIdx := leftIdx + 1

			var combined []byte
			if rightIdx < len(currentLevel) {
				combined = append(append([]byte{}, currentLevel[leftIdx]...), currentLevel[rightIdx]...)
			} else {
				// Odd element: duplicate
				combined = append(append([]byte{}, currentLevel[leftIdx]...), currentLevel[leftIdx]...)
			}
			nextLevel[i] = coinbase.DoubleSHA256(combined)
		}
		currentLevel = nextLevel
	}

	return result
}

// RegisterAddress registers a mining address for solo mode.
// Generates coinb2 for all existing jobs if this is a new address.
func (jm *JobManager) RegisterAddress(address string) {
	jm.addrMu.Lock()
	jm.connectedAddrs[address]++
	isNew := jm.connectedAddrs[address] == 1
	jm.addrMu.Unlock()

	if isNew {
		// Generate coinb2 for all existing jobs
		jm.mu.Lock()
		for _, jobData := range jm.jobs {
			if jobData.AddressCoinb2s == nil {
				jobData.AddressCoinb2s = make(map[string]string)
			}
			if _, exists := jobData.AddressCoinb2s[address]; !exists {
				c1, c2, err := jm.coin.BuildCoinbase(
					jobData.Template, address, jm.network, jm.coinbaseText,
					jm.extraNonce1Size, jm.extraNonce2Size,
				)
				if err != nil {
					jm.logger.Error("building coinbase for new address %s: %v", address, err)
					continue
				}
				jobData.AddressCoinb2s[address] = c2
				// Fix coinb1 if it was empty (initial template with no connected miners)
				if jobData.Coinb1 == "" {
					jobData.Coinb1 = c1
					jobData.Job.Coinb1 = c1
				}
			}
		}
		jm.mu.Unlock()
		jm.logger.Info("solo: registered new mining address %s", address)
	}
}

// UnregisterAddress decrements the session count for a mining address.
// Removes the address when no more sessions reference it.
func (jm *JobManager) UnregisterAddress(address string) {
	if address == "" {
		return
	}
	jm.addrMu.Lock()
	defer jm.addrMu.Unlock()

	if count, ok := jm.connectedAddrs[address]; ok {
		if count <= 1 {
			delete(jm.connectedAddrs, address)
			jm.logger.Info("solo: unregistered mining address %s (no more sessions)", address)
		} else {
			jm.connectedAddrs[address] = count - 1
		}
	}
}

// GetJobForAddress returns the latest job with the correct coinb2 for the given address.
func (jm *JobManager) GetJobForAddress(address string) *stratum.Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	// Find the most recent job
	var latest *JobData
	for _, jd := range jm.jobs {
		if latest == nil || jd.Created.After(latest.Created) {
			latest = jd
		}
	}
	if latest == nil {
		return nil
	}

	coinb2, ok := latest.AddressCoinb2s[address]
	if !ok {
		return nil
	}

	return &stratum.Job{
		JobID:          latest.Job.JobID,
		PrevHash:       latest.Job.PrevHash,
		Coinb1:         latest.Job.Coinb1,
		Coinb2:         coinb2,
		MerkleBranches: latest.Job.MerkleBranches,
		Version:        latest.Job.Version,
		NBits:          latest.Job.NBits,
		NTime:          latest.Job.NTime,
		CleanJobs:      latest.Job.CleanJobs,
	}
}

// GetAddressCoinb2 returns the coinb2 for a specific job ID and address.
func (jm *JobManager) GetAddressCoinb2(jobID, address string) (string, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	jobData, ok := jm.jobs[jobID]
	if !ok {
		return "", false
	}

	c2, ok := jobData.AddressCoinb2s[address]
	return c2, ok
}

func (jm *JobManager) pruneJobs() {
	// Find and remove the oldest jobs
	type jobAge struct {
		id      string
		created time.Time
	}
	var ages []jobAge
	for id, jd := range jm.jobs {
		ages = append(ages, jobAge{id, jd.Created})
	}

	// Simple selection: remove oldest until within limit
	for len(ages) > jm.maxJobHistory {
		oldestIdx := 0
		for i, a := range ages {
			if a.created.Before(ages[oldestIdx].created) {
				oldestIdx = i
			}
		}
		delete(jm.jobs, ages[oldestIdx].id)
		ages = append(ages[:oldestIdx], ages[oldestIdx+1:]...)
	}
}

// swapHashEndianness converts a block hash from RPC display format to the
// stratum protocol prevhash format. This matches GSS's approach:
// 1. Reverse all bytes (display big-endian → internal little-endian)
// 2. Byte-swap within each 4-byte word
// The net effect is that the 8 four-byte words are in reversed order,
// with bytes within each word in their original (display) order.
func swapHashEndianness(hashHex string) string {
	if len(hashHex) != 64 {
		return hashHex
	}

	// Step 1: Reverse all bytes (reverseHex)
	data, err := hex.DecodeString(hashHex)
	if err != nil {
		return hashHex
	}
	for i := 0; i < len(data)/2; i++ {
		data[i], data[len(data)-1-i] = data[len(data)-1-i], data[i]
	}

	// Step 2: Byte-swap within each 4-byte word (swapEndianWordsHex)
	for i := 0; i < len(data); i += 4 {
		if i+4 <= len(data) {
			data[i], data[i+3] = data[i+3], data[i]
			data[i+1], data[i+2] = data[i+2], data[i+1]
		}
	}

	return hex.EncodeToString(data)
}
