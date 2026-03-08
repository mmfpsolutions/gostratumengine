/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
	"github.com/mmfpsolutions/gostratumengine/pkg/stratum"
)

// SessionProvider returns active sessions grouped by coin symbol.
type SessionProvider func() map[string][]stratum.SessionInfo

// PoolStatsResponse is the JSON response for GET /stats.
type PoolStatsResponse struct {
	PoolName      string               `json:"pool_name"`
	UptimeSeconds float64              `json:"uptime_seconds"`
	Coins         map[string]*CoinStats `json:"coins"`
}

// MinerInfo combines live session info with historical worker stats.
type MinerInfo struct {
	WorkerName     string    `json:"worker_name"`
	RemoteAddr     string    `json:"remote_addr"`
	Difficulty     float64   `json:"difficulty"`
	ConnectedAt    time.Time `json:"connected_at"`
	SharesAccepted uint64    `json:"shares_accepted"`
	SharesRejected uint64    `json:"shares_rejected"`
	SharesStale    uint64    `json:"shares_stale"`
	BlocksFound    uint64    `json:"blocks_found"`
	LastShareTime  time.Time `json:"last_share_time,omitempty"`
	BestDifficulty float64   `json:"best_difficulty"`
}

// MinersResponse is the JSON response for GET /miners.
type MinersResponse struct {
	Miners map[string][]MinerInfo `json:"miners"`
}

// APIServer serves metrics over HTTP.
type APIServer struct {
	port            int
	poolName        string
	stats           *Stats
	sessionProvider SessionProvider
	server          *http.Server
	logger          *logging.Logger
}

// NewAPIServer creates a new metrics API server.
func NewAPIServer(port int, poolName string, stats *Stats) *APIServer {
	return &APIServer{
		port:     port,
		poolName: poolName,
		stats:    stats,
		logger:   logging.New(logging.ModuleMetrics),
	}
}

// SetSessionProvider sets the callback for retrieving active sessions.
func (a *APIServer) SetSessionProvider(sp SessionProvider) {
	a.sessionProvider = sp
}

// Start begins serving the metrics API.
func (a *APIServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stats", a.handleStats)
	mux.HandleFunc("/api/v1/miners", a.handleMiners)
	mux.HandleFunc("/api/v1/health", a.handleHealth)

	a.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.port),
		Handler: mux,
	}

	a.logger.Info("Metrics API listening on :%d", a.port)

	go func() {
		if err := a.server.ListenAndServe(); err != http.ErrServerClosed {
			a.logger.Error("metrics API error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the API server.
func (a *APIServer) Stop() {
	if a.server != nil {
		a.server.Close()
	}
	a.logger.Info("Metrics API stopped")
}

func (a *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := PoolStatsResponse{
		PoolName:      a.poolName,
		UptimeSeconds: a.stats.UptimeSeconds(),
		Coins:         a.stats.GetAllStats(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *APIServer) handleMiners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := MinersResponse{
		Miners: make(map[string][]MinerInfo),
	}

	if a.sessionProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	allSessions := a.sessionProvider()
	for symbol, sessions := range allSessions {
		workerStats := a.stats.GetWorkerStats(symbol)
		miners := make([]MinerInfo, 0, len(sessions))

		for _, sess := range sessions {
			mi := MinerInfo{
				WorkerName:  sess.WorkerName,
				RemoteAddr:  sess.RemoteAddr,
				Difficulty:  sess.Difficulty,
				ConnectedAt: sess.ConnectedAt,
			}
			if ws, ok := workerStats[sess.WorkerName]; ok {
				mi.SharesAccepted = ws.SharesAccepted
				mi.SharesRejected = ws.SharesRejected
				mi.SharesStale = ws.SharesStale
				mi.BlocksFound = ws.BlocksFound
				mi.LastShareTime = ws.LastShareTime
				mi.BestDifficulty = ws.BestDifficulty
			}
			miners = append(miners, mi)
		}
		resp.Miners[symbol] = miners
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
