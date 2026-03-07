/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	// Import coin packages to trigger init() registration
	_ "github.com/mmfpsolutions/gostratumengine/pkg/coin"

	"github.com/mmfpsolutions/gostratumengine/pkg/config"
	"github.com/mmfpsolutions/gostratumengine/pkg/engine"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/metrics"
)

var (
	version   = "dev"
	buildDate = "unknown"
	commit    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.json", "path to configuration file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("GoStratumEngine %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	logger := logging.New(logging.ModuleMain)

	// Banner
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║         GoStratumEngine                  ║")
	fmt.Printf("║         Version: %-24s║\n", version)
	fmt.Println("║         Open Source Stratum V1 Engine     ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("configuration error: %v", err)
	}

	// Set log level
	logging.SetGlobalLevel(cfg.LogLevel)
	logger.Info("pool: %s | log level: %s", cfg.PoolName, cfg.LogLevel)

	// Create stats
	stats := metrics.NewStats()

	// Create and start the engine
	eng, err := engine.New(cfg, stats)
	if err != nil {
		logger.Fatal("engine initialization: %v", err)
	}

	if err := eng.Start(); err != nil {
		logger.Fatal("engine start: %v", err)
	}

	// Start metrics API
	api := metrics.NewAPIServer(cfg.APIPort, cfg.PoolName, stats)
	api.SetSessionProvider(eng.Sessions)
	if err := api.Start(); err != nil {
		logger.Fatal("metrics API start: %v", err)
	}

	logger.Info("GoStratumEngine is running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	logger.Info("received signal %s, shutting down...", sig)

	// Graceful shutdown
	api.Stop()
	eng.Stop()

	logger.Info("GoStratumEngine stopped. Goodbye!")
}
