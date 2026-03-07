/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package stratum

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
)

// SessionState represents the state of a miner session.
type SessionState int

const (
	StateConnected SessionState = iota
	StateSubscribed
	StateAuthorized
)

// VersionRollingMask is the default mask for version rolling (BIP310).
// Allows rolling bits 13-28 (0x1fffe000).
const VersionRollingMask = "1fffe000"

// Session represents a single miner connection.
type Session struct {
	ID           string
	conn         net.Conn
	server       *Server
	state        SessionState
	workerName   string
	extraNonce1  string
	difficulty   float64
	vardiff      *VarDiff
	logger       *logging.Logger
	mu           sync.Mutex
	closed       bool
	connectedAt  time.Time
	lastActivity time.Time

	// Version rolling
	versionRollingEnabled bool
	versionRollingMask    uint32

	// Solo mining
	miningAddress string
}

// newSession creates a new miner session.
func newSession(id string, conn net.Conn, extraNonce1 string, server *Server) *Session {
	return &Session{
		ID:           id,
		conn:         conn,
		server:       server,
		state:        StateConnected,
		extraNonce1:  extraNonce1,
		difficulty:   server.defaultDiff,
		logger:       logging.New(logging.ModuleStratum),
		connectedAt:  time.Now(),
		lastActivity: time.Now(),
	}
}

// Run processes messages from the miner until the connection closes.
func (s *Session) Run() {
	defer s.server.removeSession(s.ID)
	defer s.conn.Close()

	scanner := bufio.NewScanner(s.conn)
	scanner.Buffer(make([]byte, 0, 16*1024), 16*1024)

	// Set initial read deadline
	s.conn.SetReadDeadline(time.Now().Add(s.server.idleTimeout))

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		s.lastActivity = time.Now()
		s.conn.SetReadDeadline(time.Now().Add(s.server.idleTimeout))

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Debug("[%s] malformed JSON from miner: %v", s.ID, err)
			continue
		}

		// Skip responses (pong replies to our pings) — no method field
		if req.Method == "" {
			continue
		}

		s.handleRequest(&req)
	}

	if err := scanner.Err(); err != nil {
		if !s.closed {
			s.logger.Debug("[%s] connection read error: %v", s.ID, err)
		}
	}
}

func (s *Session) handleRequest(req *Request) {
	switch req.Method {
	case "mining.subscribe":
		s.handleSubscribe(req)
	case "mining.authorize":
		s.handleAuthorize(req)
	case "mining.submit":
		s.handleSubmit(req)
	case "mining.suggest_difficulty":
		s.handleSuggestDifficulty(req)
	case "mining.configure":
		s.handleConfigure(req)
	case "mining.ping":
		// Some miners send mining.ping — respond with pong
		s.sendResult(req.ID, "pong")
	case "pong":
		// Miner responding to our mining.ping — acknowledge it
		s.sendResult(req.ID, true)
	case "mining.extranonce.subscribe":
		// Some miners request extranonce subscribe — acknowledge it
		s.sendResult(req.ID, true)
	default:
		s.logger.Debug("[%s] unknown method: %s", s.ID, req.Method)
		s.sendError(req.ID, ErrOther)
	}
}

func (s *Session) handleSubscribe(req *Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateSubscribed

	extraNonce2Size := s.server.extraNonceSize - len(s.extraNonce1)/2
	result := SubscribeResult(s.ID, s.extraNonce1, extraNonce2Size)
	s.sendResult(req.ID, result)

	s.logger.Debug("[%s] subscribed", s.ID)
}

func (s *Session) handleAuthorize(req *Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state < StateSubscribed {
		s.sendError(req.ID, ErrNotSubscribed)
		return
	}

	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		s.sendError(req.ID, ErrMalformedRequest)
		return
	}

	s.workerName = params[0]

	// Call authorize handler if set (solo mode uses this for address validation)
	if s.server.authorizeHandler != nil {
		miningAddr, err := s.server.authorizeHandler(s, s.workerName)
		if err != nil {
			s.logger.Warn("[%s] authorize rejected for %s: %v", s.ID, s.workerName, err)
			s.sendError(req.ID, ErrUnauthorized)
			return
		}
		s.miningAddress = miningAddr
	}

	s.state = StateAuthorized

	// Send authorize success
	s.sendResult(req.ID, true)

	// Send initial difficulty
	s.sendJSON(SetDifficultyNotification(s.difficulty))

	// Send current job if available
	if s.server.jobForSessionHandler != nil {
		if job := s.server.jobForSessionHandler(s); job != nil {
			s.sendJSON(NotifyNotification(job))
		}
	} else if job := s.server.getCurrentJob(); job != nil {
		s.sendJSON(NotifyNotification(job))
	}

	s.logger.Info("[%s] authorized worker: %s (diff: %.2f)", s.ID, s.workerName, s.difficulty)
}

func (s *Session) handleSubmit(req *Request) {
	if s.state < StateAuthorized {
		s.sendError(req.ID, ErrUnauthorized)
		return
	}

	share, err := ParseShareSubmission(req.Params)
	if err != nil {
		s.sendError(req.ID, ErrMalformedRequest)
		return
	}

	share.WorkerName = s.workerName

	// Delegate to server's share handler
	if err := s.server.handleShare(s, share); err != nil {
		if sErr, ok := err.(stratumError); ok {
			s.sendError(req.ID, sErr)
		} else {
			s.sendError(req.ID, ErrOther)
		}
		return
	}

	s.sendResult(req.ID, true)

	// Check vardiff after successful share
	if s.vardiff != nil {
		if newDiff := s.vardiff.RecordShare(); newDiff > 0 {
			s.mu.Lock()
			s.difficulty = newDiff
			s.mu.Unlock()
			s.sendJSON(SetDifficultyNotification(newDiff))
			s.logger.Debug("[%s] vardiff adjusted to %.2f for %s", s.ID, newDiff, s.workerName)
		}
	}
}

func (s *Session) handleSuggestDifficulty(req *Request) {
	// Like GSS, ignore mining.suggest_difficulty — the pool's configured difficulty
	// and VarDiff are the authority. Accepting miner-suggested difficulty can cause
	// problems (e.g., Bitaxe firmware re-sending after every ping, undoing VarDiff).
	var params []float64
	if err := json.Unmarshal(req.Params, &params); err == nil && len(params) > 0 {
		s.logger.Debug("[%s] mining.suggest_difficulty ignored (requested: %.2f, using pool diff: %.2f)", s.ID, params[0], s.difficulty)
	}
	// Respond with true to acknowledge (don't error)
	s.sendResult(req.ID, true)
}

func (s *Session) handleConfigure(req *Request) {
	var params []json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 2 {
		s.sendError(req.ID, ErrMalformedRequest)
		return
	}

	var extensions []string
	if err := json.Unmarshal(params[0], &extensions); err != nil {
		s.sendError(req.ID, ErrMalformedRequest)
		return
	}

	result := make(map[string]interface{})

	for _, ext := range extensions {
		if ext == "version-rolling" {
			s.mu.Lock()
			s.versionRollingEnabled = true
			s.versionRollingMask = 0x1fffe000 // Standard mask
			s.mu.Unlock()

			result["version-rolling"] = true
			result["version-rolling.mask"] = VersionRollingMask
			result["version-rolling.min-bit-count"] = 2
		}
	}

	s.sendResult(req.ID, result)
	s.logger.Debug("[%s] configure: version-rolling=%v", s.ID, s.versionRollingEnabled)
}

// SendPing sends a mining.ping request to the miner with a numeric ID.
func (s *Session) SendPing(id uint64) {
	s.sendJSON(PingRequest(id))
}

// SendJob sends a mining.notify notification to the miner.
func (s *Session) SendJob(job *Job) {
	if s.state < StateAuthorized {
		return
	}
	s.sendJSON(NotifyNotification(job))
}

func (s *Session) sendResult(id json.RawMessage, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   nil,
	}
	s.sendJSON(resp)
}

func (s *Session) sendError(id json.RawMessage, err stratumError) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  nil,
		Error:   err.ToJSON(),
	}
	s.sendJSON(resp)
}

func (s *Session) sendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("[%s] marshal error: %v", s.ID, err)
		return
	}
	data = append(data, '\n')

	s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := s.conn.Write(data); err != nil {
		if !s.closed {
			s.logger.Debug("[%s] write error: %v", s.ID, err)
		}
	}
}

// MiningAddress returns the session's validated mining address (solo mode).
func (s *Session) MiningAddress() string {
	return s.miningAddress
}

// ExtraNonce1 returns the session's extranonce1 value.
func (s *Session) ExtraNonce1() string {
	return s.extraNonce1
}

// GetDifficulty returns the session's current difficulty.
func (s *Session) GetDifficulty() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.difficulty
}

// Close marks the session as closed and closes the underlying connection.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.conn.Close()
	}
}

// Info returns a summary of the session for metrics/API.
func (s *Session) Info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionInfo{
		ID:          s.ID,
		WorkerName:  s.workerName,
		RemoteAddr:  s.conn.RemoteAddr().String(),
		Difficulty:  s.difficulty,
		ConnectedAt: s.connectedAt,
		State:       fmt.Sprintf("%d", s.state),
	}
}

// SessionInfo holds session metadata for external consumption.
type SessionInfo struct {
	ID          string    `json:"id"`
	WorkerName  string    `json:"worker_name"`
	RemoteAddr  string    `json:"remote_addr"`
	Difficulty  float64   `json:"difficulty"`
	ConnectedAt time.Time `json:"connected_at"`
	State       string    `json:"state"`
}
