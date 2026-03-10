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
	"strconv"
	"strings"
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
	difficulty    float64
	prevDiff      float64   // previous difficulty before last change
	diffChangedAt time.Time // when difficulty last changed
	vardiff       *VarDiff
	logger       *logging.Logger
	mu           sync.Mutex
	closed       bool
	connectedAt  time.Time
	lastActivity time.Time

	// Version rolling
	versionRollingEnabled bool
	versionRollingMask    uint32

	// Pending difficulty change (applied on next job broadcast)
	pendingDiff float64

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

	// Parse password-based difficulty: "d=512" or "d=1024" in params[1]
	if len(params) >= 2 {
		s.parsePasswordDifficulty(params[1])
	}

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

	// Check vardiff after successful share — store pending diff for next job broadcast.
	// Skip if a difficulty change is already pending — shares at the old difficulty
	// would produce increasingly wrong calculations while waiting for delivery.
	if s.vardiff != nil {
		s.mu.Lock()
		hasPending := s.pendingDiff > 0
		s.mu.Unlock()
		if hasPending {
			s.logger.Debug("[%s] %s VARDIFF: skipped (pending diff %.4f waiting for delivery)",
				s.ID, s.workerName, s.pendingDiff)
		} else {
			result := s.vardiff.RecordShare()
			if diag := result.DiagString(); diag != "" {
				if result.Adjusted {
					s.logger.Debug("[%s] %s VARDIFF: adjusted %.4f -> %.4f | %s",
						s.ID, s.workerName, result.CurrentDiff, result.ClampedDiff, diag)
				} else {
					s.logger.Debug("[%s] %s VARDIFF: no adjustment - %s (difficulty stays at %.4f) | %s",
						s.ID, s.workerName, result.Reason, result.CurrentDiff, diag)
				}
			}
			if result.Adjusted {
				if !s.server.vardiffOnNewBlock {
					// Mid-block: send difficulty change immediately with job resend
					s.sendMidBlockDifficulty(result.ClampedDiff)
				} else {
					// Queue for next job boundary (new block)
					s.mu.Lock()
					s.pendingDiff = result.ClampedDiff
					s.mu.Unlock()
				}
			}
		}
	}
}

func (s *Session) handleSuggestDifficulty(req *Request) {
	var params []float64
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		s.sendResult(req.ID, true)
		return
	}

	suggested := params[0]

	if !s.server.acceptSuggestDiff {
		s.logger.Debug("[%s] mining.suggest_difficulty ignored (requested: %.2f, using pool diff: %.2f)", s.ID, suggested, s.difficulty)
		s.sendResult(req.ID, true)
		return
	}

	// Clamp to vardiff bounds if configured, otherwise use as-is
	newDiff := suggested
	if s.vardiff != nil {
		if newDiff < s.vardiff.minDiff {
			newDiff = s.vardiff.minDiff
		}
		if newDiff > s.vardiff.maxDiff {
			newDiff = s.vardiff.maxDiff
		}
	}
	if newDiff <= 0 {
		newDiff = s.server.defaultDiff
	}

	s.mu.Lock()
	s.prevDiff = s.difficulty
	s.diffChangedAt = time.Now()
	s.difficulty = newDiff
	s.mu.Unlock()

	s.sendJSON(SetDifficultyNotification(newDiff))
	s.logger.Info("[%s] mining.suggest_difficulty accepted: requested=%.2f set=%.2f", s.ID, suggested, newDiff)
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

// parsePasswordDifficulty checks the authorize password field for "d=XXX"
// and sets the session difficulty if found. Clamped to vardiff bounds.
func (s *Session) parsePasswordDifficulty(password string) {
	for _, part := range strings.Split(password, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "d=") {
			val, err := strconv.ParseFloat(part[2:], 64)
			if err != nil || val <= 0 {
				continue
			}
			// Clamp to vardiff bounds if configured
			if s.vardiff != nil {
				if val < s.vardiff.minDiff {
					val = s.vardiff.minDiff
				}
				if val > s.vardiff.maxDiff {
					val = s.vardiff.maxDiff
				}
			}
			s.prevDiff = s.difficulty
			s.diffChangedAt = time.Now()
			s.difficulty = val
			s.logger.Info("[%s] password difficulty set to %.2f for %s", s.ID, val, s.workerName)
			return
		}
	}
}

// SendPing sends a mining.ping request to the miner with a numeric ID.
func (s *Session) SendPing(id uint64) {
	s.sendJSON(PingRequest(id))
}

// SendJob sends a mining.notify notification to the miner.
// If a vardiff adjustment is pending, set_difficulty is sent first.
func (s *Session) SendJob(job *Job) {
	if s.state < StateAuthorized {
		return
	}

	// Flush pending vardiff. When vardiffOnNewBlock is true (default),
	// only apply on clean jobs (new blocks) to avoid low-diff shares mid-block.
	s.mu.Lock()
	flushDiff := s.pendingDiff > 0 && (!s.server.vardiffOnNewBlock || job.CleanJobs)
	if flushDiff {
		newDiff := s.pendingDiff
		s.prevDiff = s.difficulty
		s.diffChangedAt = time.Now()
		s.difficulty = newDiff
		s.pendingDiff = 0
		s.mu.Unlock()
		s.sendJSON(SetDifficultyNotification(newDiff))
		// Reset vardiff window so new difficulty starts with clean measurements
		if s.vardiff != nil {
			s.vardiff.ResetWindow(newDiff)
		}
		s.logger.Debug("[%s] vardiff applied %.2f for %s", s.ID, newDiff, s.workerName)
	} else {
		s.mu.Unlock()
	}

	s.sendJSON(NotifyNotification(job))
}

// sendMidBlockDifficulty sends a difficulty change immediately without waiting
// for the next block. Resends the current job with clean_jobs=false so ASIC
// miners (e.g., bitaxe) acknowledge the new difficulty without wasting hash power.
func (s *Session) sendMidBlockDifficulty(newDiff float64) {
	oldDiff := s.difficulty

	// Update session state
	s.mu.Lock()
	s.prevDiff = s.difficulty
	s.diffChangedAt = time.Now()
	s.difficulty = newDiff
	s.pendingDiff = 0
	s.mu.Unlock()

	// Send mining.set_difficulty
	s.sendJSON(SetDifficultyNotification(newDiff))

	// Reset vardiff window so new difficulty starts with clean measurements
	if s.vardiff != nil {
		s.vardiff.ResetWindow(newDiff)
	}

	// Resend current job with clean_jobs=false so the miner applies the new
	// difficulty without discarding work on the current block template
	if currentJob := s.server.getCurrentJob(); currentJob != nil {
		resendJob := *currentJob
		resendJob.CleanJobs = false
		s.sendJSON(NotifyNotification(&resendJob))
	}

	s.logger.Info("[%s] %s VARDIFF: mid-block difficulty %.4f -> %.4f",
		s.ID, s.workerName, oldDiff, newDiff)
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

// GetPrevDifficulty returns the previous difficulty and when it changed.
// Used by the share validator for the low-diff grace period.
func (s *Session) GetPrevDifficulty() (float64, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prevDiff, s.diffChangedAt
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
