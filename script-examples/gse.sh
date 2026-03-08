#!/bin/bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#
# gse.sh — Simple init-style script for GoStratumEngine.
# Place this script alongside the gostratumengine binary and config.json.
# Usage: ./gse.sh {start|stop|restart|status}
#

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/gostratumengine"
CONFIG="$SCRIPT_DIR/config.json"
LOGFILE="$SCRIPT_DIR/gse.log"
PIDFILE="$SCRIPT_DIR/gse.pid"

start() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "gse is already running (PID $(cat "$PIDFILE"))"
        exit 1
    fi

    echo "Starting gse..."
    nohup "$BINARY" -config "$CONFIG" >> "$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    echo "gse started (PID $(cat "$PIDFILE"))"
}

stop() {
    if [ ! -f "$PIDFILE" ]; then
        echo "gse is not running (no PID file found)"
        exit 1
    fi

    PID=$(cat "$PIDFILE")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Stopping gse (PID $PID)..."
        kill "$PID"
        rm -f "$PIDFILE"
        echo "gse stopped"
    else
        echo "gse is not running (stale PID file removed)"
        rm -f "$PIDFILE"
        exit 1
    fi
}

status() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "gse is running (PID $(cat "$PIDFILE"))"
    else
        echo "gse is not running"
    fi
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    status)  status ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
