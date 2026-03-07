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
	"fmt"
	"strings"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/coin"
	"github.com/mmfpsolutions/gostratumengine/pkg/config"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/metrics"
	"github.com/mmfpsolutions/gostratumengine/pkg/noderpc"
	"github.com/mmfpsolutions/gostratumengine/pkg/stratum"
)

// CoinRunner manages the complete mining pipeline for a single coin.
type CoinRunner struct {
	symbol    string
	coin      coin.Coin
	rpcClient *noderpc.Client
	jobMgr    *JobManager
	validator *ShareValidator
	server    *stratum.Server
	zmqSub    *noderpc.ZMQSubscriber
	stats     *metrics.Stats
	logger    *logging.Logger
}

// NewCoinRunner creates and wires up all components for a single coin.
func NewCoinRunner(symbol string, cfg config.CoinConfig, stats *metrics.Stats) (*CoinRunner, error) {
	soloMode := cfg.Mining.Mode == "solo"

	// Look up coin implementation
	c, err := coin.Get(cfg.CoinType)
	if err != nil {
		return nil, fmt.Errorf("coin type %s: %w", cfg.CoinType, err)
	}

	// Validate mining address (required in pool mode, optional in solo mode)
	if !soloMode {
		if err := c.ValidateAddress(cfg.Mining.Address, cfg.Mining.Network); err != nil {
			return nil, fmt.Errorf("invalid mining address for %s: %w", symbol, err)
		}
	}

	// Create RPC client
	rpcClient := noderpc.NewClient(
		cfg.Node.Host, cfg.Node.Port,
		cfg.Node.Username, cfg.Node.Password,
	)

	runner := &CoinRunner{
		symbol:    symbol,
		coin:      c,
		rpcClient: rpcClient,
		stats:     stats,
		logger:    logging.New(logging.ModuleEngine),
	}

	// Create stratum server
	var vardiffCfg *stratum.VarDiffConfig
	if cfg.VarDiff.Enabled {
		vardiffCfg = &stratum.VarDiffConfig{
			MinDiff:        cfg.VarDiff.MinDiff,
			MaxDiff:        cfg.VarDiff.MaxDiff,
			TargetTime:     cfg.VarDiff.TargetTime,
			RetargetTime:   cfg.VarDiff.RetargetTime,
			VariancePct:    cfg.VarDiff.VariancePct,
			FloatDiff:      cfg.VarDiff.FloatDiff,
			FloatPrecision: cfg.VarDiff.FloatPrecision,
		}
	}

	// Create job manager
	jobMgr := NewJobManager(JobManagerConfig{
		Coin:           c,
		RPCClient:      rpcClient,
		Address:        cfg.Mining.Address,
		Network:        cfg.Mining.Network,
		CoinbaseText:   cfg.Mining.CoinbaseText,
		ExtraNonceSize: cfg.Mining.ExtraNonceSize,
		PollInterval:   time.Duration(cfg.TemplateRefreshInterval) * time.Second,
		SoloMode:       soloMode,
	})
	runner.jobMgr = jobMgr

	// Create share validator
	validator := NewShareValidator(c, jobMgr, rpcClient, stats, soloMode)
	runner.validator = validator

	// Wire share handler: stratum server calls validator
	shareHandler := func(session *stratum.Session, share *stratum.ShareSubmission) error {
		return validator.ValidateShare(session, share)
	}

	// Build server config
	serverCfg := stratum.ServerConfig{
		Host:           cfg.Stratum.Host,
		Port:           cfg.Stratum.Port,
		ExtraNonceSize: cfg.Mining.ExtraNonceSize,
		DefaultDiff:    cfg.Stratum.Difficulty,
		PingEnabled:    cfg.Stratum.PingEnabled,
		PingInterval:   time.Duration(cfg.Stratum.PingInterval) * time.Second,
		IdleTimeout:    5 * time.Minute,
		VarDiff:        vardiffCfg,
	}

	// Solo mode: set up authorize, job-for-session, and disconnect handlers
	if soloMode {
		serverCfg.AuthorizeHandler = func(session *stratum.Session, workerName string) (string, error) {
			// Parse address from workerName: "address.workerID" -> "address"
			address := workerName
			if dotIdx := strings.Index(workerName, "."); dotIdx > 0 {
				address = workerName[:dotIdx]
			}

			// Validate the address
			if err := c.ValidateAddress(address, cfg.Mining.Network); err != nil {
				return "", fmt.Errorf("invalid mining address: %w", err)
			}

			// Register this address with the job manager
			jobMgr.RegisterAddress(address)

			return address, nil
		}

		serverCfg.JobForSessionHandler = func(session *stratum.Session) *stratum.Job {
			return jobMgr.GetJobForAddress(session.MiningAddress())
		}

		serverCfg.OnSessionRemoved = func(session *stratum.Session) {
			if addr := session.MiningAddress(); addr != "" {
				jobMgr.UnregisterAddress(addr)
			}
		}
	}

	server := stratum.NewServer(serverCfg, shareHandler)
	runner.server = server

	// Wire job manager broadcast
	if soloMode {
		jobMgr.onNewJob = func(job *stratum.Job) {
			server.BroadcastJobPerSession(job, func(session *stratum.Session) *stratum.Job {
				addr := session.MiningAddress()
				if addr == "" {
					return nil
				}
				coinb2, ok := jobMgr.GetAddressCoinb2(job.JobID, addr)
				if !ok {
					return nil
				}
				return &stratum.Job{
					JobID:          job.JobID,
					PrevHash:       job.PrevHash,
					Coinb1:         job.Coinb1,
					Coinb2:         coinb2,
					MerkleBranches: job.MerkleBranches,
					Version:        job.Version,
					NBits:          job.NBits,
					NTime:          job.NTime,
					CleanJobs:      job.CleanJobs,
				}
			})
		}
	} else {
		jobMgr.onNewJob = func(job *stratum.Job) {
			server.BroadcastJob(job)
		}
	}

	// Set up ZMQ if enabled
	if cfg.Node.ZMQEnabled && cfg.Node.ZMQHashBlock != "" {
		runner.zmqSub = noderpc.NewZMQSubscriber(cfg.Node.ZMQHashBlock, func(blockHash string) {
			jobMgr.OnBlockNotification(blockHash)
		})
	}

	return runner, nil
}

// Start begins the coin mining pipeline.
func (cr *CoinRunner) Start() error {
	// Test RPC connection
	if err := cr.rpcClient.Ping(); err != nil {
		return fmt.Errorf("%s: cannot connect to node: %w", cr.symbol, err)
	}

	info, err := cr.rpcClient.GetBlockchainInfo()
	if err != nil {
		return fmt.Errorf("%s: getblockchaininfo: %w", cr.symbol, err)
	}
	cr.logger.Info("[%s] connected to %s node (chain: %s, height: %d)",
		cr.symbol, cr.coin.Name(), info.Chain, info.Blocks)

	// Initialize stats for this coin
	cr.stats.InitCoin(cr.symbol)

	// Start stratum server
	if err := cr.server.Start(); err != nil {
		return fmt.Errorf("%s: starting stratum: %w", cr.symbol, err)
	}

	// Start job manager (fetches first template and begins polling)
	if err := cr.jobMgr.Start(); err != nil {
		cr.server.Stop()
		return fmt.Errorf("%s: starting job manager: %w", cr.symbol, err)
	}

	// Start ZMQ subscriber if configured
	if cr.zmqSub != nil {
		if err := cr.zmqSub.Start(); err != nil {
			cr.logger.Warn("[%s] ZMQ failed to start, falling back to polling: %v", cr.symbol, err)
			cr.zmqSub = nil
		}
	}

	cr.logger.Info("[%s] coin runner started", cr.symbol)
	return nil
}

// Stop shuts down the coin mining pipeline.
func (cr *CoinRunner) Stop() {
	if cr.zmqSub != nil {
		cr.zmqSub.Stop()
	}
	cr.jobMgr.Stop()
	cr.server.Stop()
	cr.logger.Info("[%s] coin runner stopped", cr.symbol)
}

// SessionCount returns the number of active miner connections.
func (cr *CoinRunner) SessionCount() int {
	return cr.server.SessionCount()
}

// Sessions returns info for all active sessions on this coin.
func (cr *CoinRunner) Sessions() []stratum.SessionInfo {
	return cr.server.Sessions()
}
