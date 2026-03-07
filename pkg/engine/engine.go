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

	"github.com/mmfpsolutions/gostratumengine/pkg/config"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/metrics"
	"github.com/mmfpsolutions/gostratumengine/pkg/stratum"
)

// Engine is the top-level orchestrator that manages all coin runners.
type Engine struct {
	runners map[string]*CoinRunner
	stats   *metrics.Stats
	logger  *logging.Logger
}

// New creates a new Engine from the given configuration.
func New(cfg *config.Config, stats *metrics.Stats) (*Engine, error) {
	e := &Engine{
		runners: make(map[string]*CoinRunner),
		stats:   stats,
		logger:  logging.New(logging.ModuleEngine),
	}

	for symbol, coinCfg := range cfg.Coins {
		if !coinCfg.Enabled {
			e.logger.Info("[%s] skipped (disabled)", symbol)
			continue
		}

		runner, err := NewCoinRunner(symbol, coinCfg, stats)
		if err != nil {
			return nil, fmt.Errorf("initializing %s: %w", symbol, err)
		}
		e.runners[symbol] = runner
	}

	if len(e.runners) == 0 {
		return nil, fmt.Errorf("no coins initialized")
	}

	return e, nil
}

// Start begins all coin runners.
func (e *Engine) Start() error {
	var started []string

	for symbol, runner := range e.runners {
		if err := runner.Start(); err != nil {
			// Stop any runners that already started
			for _, s := range started {
				e.runners[s].Stop()
			}
			return fmt.Errorf("starting %s: %w", symbol, err)
		}
		started = append(started, symbol)
	}

	e.logger.Info("engine started with %d coin(s)", len(e.runners))
	return nil
}

// Stop shuts down all coin runners.
func (e *Engine) Stop() {
	for symbol, runner := range e.runners {
		e.logger.Info("stopping %s...", symbol)
		runner.Stop()
	}
	e.logger.Info("engine stopped")
}

// Stats returns the metrics stats instance.
func (e *Engine) Stats() *metrics.Stats {
	return e.stats
}

// RunnerCount returns the number of active coin runners.
func (e *Engine) RunnerCount() int {
	return len(e.runners)
}

// Sessions returns all active sessions grouped by coin symbol.
func (e *Engine) Sessions() map[string][]stratum.SessionInfo {
	result := make(map[string][]stratum.SessionInfo)
	for symbol, runner := range e.runners {
		result[symbol] = runner.Sessions()
	}
	return result
}
