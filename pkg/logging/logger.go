/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package logging

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var (
	globalLevel Level = LevelInfo
	globalMu    sync.RWMutex
)

// Module constants for consistent tagging.
const (
	ModuleMain    = "main"
	ModuleStratum = "stratum"
	ModuleEngine  = "engine"
	ModuleCoin    = "coin"
	ModuleRPC     = "rpc"
	ModuleMetrics = "metrics"
	ModuleConfig  = "config"
	ModuleZMQ     = "zmq"
)

// Logger provides module-tagged leveled logging to stdout.
type Logger struct {
	module string
}

// New creates a new Logger with the given module tag.
func New(module string) *Logger {
	return &Logger{module: module}
}

// SetGlobalLevel sets the minimum log level for all loggers.
func SetGlobalLevel(level string) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLevel = ParseLevel(level)
}

// GetGlobalLevel returns the current global log level.
func GetGlobalLevel() Level {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLevel
}

// ParseLevel converts a string to a Level. Defaults to LevelInfo for unknown values.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < GetGlobalLevel() {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "[%s] [%s] [%s] %s\n", timestamp, l.module, level, msg)

	if level == LevelFatal {
		os.Exit(1)
	}
}

// Debug logs a message at DEBUG level.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs a message at INFO level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a message at WARN level.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs a message at ERROR level.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Fatal logs a message at FATAL level and exits the process.
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelFatal, format, args...)
}
