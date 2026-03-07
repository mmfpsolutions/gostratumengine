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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
)

// ShareHandler is called when a miner submits a share.
type ShareHandler func(session *Session, share *ShareSubmission) error

// AuthorizeHandler is called during mining.authorize to validate the worker.
// Returns the mining address to use and an error if authorization fails.
type AuthorizeHandler func(session *Session, workerName string) (miningAddress string, err error)

// JobForSessionHandler returns the current job customized for a session.
type JobForSessionHandler func(session *Session) *Job

// Server is a Stratum V1 TCP server that manages miner sessions.
type Server struct {
	addr                 string
	listener             net.Listener
	sessions             map[string]*Session
	sessionsMu           sync.RWMutex
	currentJob           atomic.Pointer[Job]
	shareHandler         ShareHandler
	authorizeHandler     AuthorizeHandler
	jobForSessionHandler JobForSessionHandler
	onSessionRemoved     func(session *Session)
	extraNonceSize       int
	defaultDiff          float64
	pingEnabled          bool
	pingInterval         time.Duration
	idleTimeout          time.Duration
	vardiffCfg           *VarDiffConfig
	logger               *logging.Logger
	extraNonceSeq        atomic.Uint32
	pingIDSeq            atomic.Uint64
	shutdownCh           chan struct{}
	wg                   sync.WaitGroup
}

// ServerConfig holds configuration for the stratum server.
type ServerConfig struct {
	Host                 string
	Port                 int
	ExtraNonceSize       int
	DefaultDiff          float64
	PingEnabled          bool
	PingInterval         time.Duration
	IdleTimeout          time.Duration
	VarDiff              *VarDiffConfig
	AuthorizeHandler     AuthorizeHandler
	JobForSessionHandler JobForSessionHandler
	OnSessionRemoved     func(session *Session)
}

// NewServer creates a new Stratum server.
func NewServer(cfg ServerConfig, shareHandler ShareHandler) *Server {
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	return &Server{
		addr:                 fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		sessions:             make(map[string]*Session),
		shareHandler:         shareHandler,
		authorizeHandler:     cfg.AuthorizeHandler,
		jobForSessionHandler: cfg.JobForSessionHandler,
		onSessionRemoved:     cfg.OnSessionRemoved,
		extraNonceSize:       cfg.ExtraNonceSize,
		defaultDiff:          cfg.DefaultDiff,
		pingEnabled:          cfg.PingEnabled,
		pingInterval:         cfg.PingInterval,
		idleTimeout:          cfg.IdleTimeout,
		vardiffCfg:           cfg.VarDiff,
		logger:               logging.New(logging.ModuleStratum),
		shutdownCh:           make(chan struct{}),
	}
}

// Start begins listening for miner connections.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	s.listener = listener

	s.logger.Info("Stratum server listening on %s", s.addr)

	s.wg.Add(1)
	go s.acceptLoop()

	// Start ping loop if enabled
	if s.pingEnabled && s.pingInterval > 0 {
		s.wg.Add(1)
		go s.pingLoop()
	}

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdownCh:
				return
			default:
				s.logger.Error("accept error: %v", err)
				continue
			}
		}

		// Configure TCP connection
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(30 * time.Second)
			tcpConn.SetNoDelay(true)
		}

		sessionID := s.generateSessionID()
		extraNonce1 := s.generateExtraNonce1()

		session := newSession(sessionID, conn, extraNonce1, s)

		// Set up vardiff if configured
		if s.vardiffCfg != nil {
			session.vardiff = NewVarDiff(*s.vardiffCfg, s.defaultDiff)
		}

		s.addSession(session)

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			session.Run()
		}()

		s.logger.Debug("new connection from %s (session: %s)", conn.RemoteAddr(), sessionID)
	}
}

// Stop gracefully shuts down the stratum server.
func (s *Server) Stop() {
	close(s.shutdownCh)
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all sessions
	s.sessionsMu.RLock()
	for _, session := range s.sessions {
		session.Close()
	}
	s.sessionsMu.RUnlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.logger.Warn("stratum server shutdown timed out")
	}

	s.logger.Info("Stratum server stopped")
}

// BroadcastJob sends a new job to all authorized sessions.
func (s *Server) BroadcastJob(job *Job) {
	s.currentJob.Store(job)

	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	for _, session := range s.sessions {
		session.SendJob(job)
	}
}

// BroadcastJobPerSession sends a customized job to each session.
// The customizer function returns the job variant for each session.
func (s *Server) BroadcastJobPerSession(baseJob *Job, customizer func(*Session) *Job) {
	s.currentJob.Store(baseJob)

	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	for _, session := range s.sessions {
		if session.state < StateAuthorized {
			continue
		}
		if job := customizer(session); job != nil {
			session.SendJob(job)
		}
	}
}

func (s *Server) getCurrentJob() *Job {
	return s.currentJob.Load()
}

func (s *Server) handleShare(session *Session, share *ShareSubmission) error {
	if s.shareHandler == nil {
		return ErrOther
	}
	return s.shareHandler(session, share)
}

func (s *Server) addSession(session *Session) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.sessions[session.ID] = session
}

func (s *Server) removeSession(id string) {
	s.sessionsMu.Lock()
	session, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.sessionsMu.Unlock()

	if ok {
		s.logger.Debug("[%s] disconnected (worker: %s)", id, session.workerName)
		if s.onSessionRemoved != nil {
			s.onSessionRemoved(session)
		}
	}
}

// SessionCount returns the number of active sessions.
func (s *Server) SessionCount() int {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return len(s.sessions)
}

// Sessions returns info for all active sessions.
func (s *Server) Sessions() []SessionInfo {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	infos := make([]SessionInfo, 0, len(s.sessions))
	for _, session := range s.sessions {
		infos = append(infos, session.Info())
	}
	return infos
}

func (s *Server) generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) pingLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCh:
			return
		case <-ticker.C:
			s.sessionsMu.RLock()
			for _, session := range s.sessions {
				if session.state >= StateAuthorized {
						id := s.pingIDSeq.Add(1) + 9 // Start at 10
				session.SendPing(id)
				}
			}
			s.sessionsMu.RUnlock()
		}
	}
}

func (s *Server) generateExtraNonce1() string {
	seq := s.extraNonceSeq.Add(1)
	// Use 4 bytes for extranonce1 (enough for 4 billion miners)
	b := []byte{byte(seq >> 24), byte(seq >> 16), byte(seq >> 8), byte(seq)}
	return hex.EncodeToString(b)
}
