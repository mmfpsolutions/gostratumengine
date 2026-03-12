#!/usr/bin/env bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIDFILE="$SCRIPT_DIR/dashboard.pid"
LOGFILE="$SCRIPT_DIR/dashboard.log"
SERVER="$SCRIPT_DIR/server.py"

start() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "Dashboard is already running (PID $(cat "$PIDFILE"))."
        return 1
    fi
    echo "Starting dashboard..."
    nohup python3 "$SERVER" >> "$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    echo "Dashboard started (PID $!)."
    echo "Log: $LOGFILE"
}

stop() {
    if [ ! -f "$PIDFILE" ]; then
        echo "Dashboard is not running (no PID file)."
        return 1
    fi
    PID=$(cat "$PIDFILE")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Stopping dashboard (PID $PID)..."
        kill "$PID"
        rm -f "$PIDFILE"
        echo "Dashboard stopped."
    else
        echo "Dashboard is not running (stale PID file). Cleaning up."
        rm -f "$PIDFILE"
    fi
}

status() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "Dashboard is running (PID $(cat "$PIDFILE"))."
    else
        echo "Dashboard is not running."
        [ -f "$PIDFILE" ] && rm -f "$PIDFILE"
    fi
}

case "${1:-}" in
    start)  start  ;;
    stop)   stop   ;;
    status) status ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac
