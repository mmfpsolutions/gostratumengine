#!/bin/bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#
# service.sh — Start/stop/restart/status for Bitcoin Cash node.
# Usage: ./service.sh {start|stop|restart|status}

# Configuration
DATADIR="{BASE_PATH}/bch/data"
PIDFILE="$DATADIR/bitcoind.pid"
DAEMON="{BASE_PATH}/bch/bitcoind"
CLI="{BASE_PATH}/bch/bitcoin-cli"

if [ ! -d "$DATADIR" ]; then
    echo "Error: Data directory $DATADIR does not exist."
    echo "Please create it and ensure proper permissions before running this script."
    exit 1
fi

start() {
    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "bitcoind (BCH) is already running (PID $PID)."
            exit 1
        else
            echo "Stale PID file found. Cleaning up."
            rm -f "$PIDFILE"
        fi
    fi

    echo "Starting bitcoind (BCH)..."
    $DAEMON -datadir="$DATADIR" -daemon
    sleep 2
    if [ -f "$PIDFILE" ]; then
        echo "bitcoind (BCH) started (PID $(cat "$PIDFILE"))."
    else
        echo "bitcoind (BCH) started."
    fi
}

stop() {
    if [ ! -f "$PIDFILE" ]; then
        echo "bitcoind (BCH) is not running (no PID file)."
        exit 1
    fi

    PID=$(cat "$PIDFILE")
    echo "Stopping bitcoind (BCH) (PID $PID)..."

    # Use bitcoin-cli stop for graceful shutdown if available
    if [ -x "$CLI" ]; then
        $CLI -datadir="$DATADIR" stop 2>/dev/null || kill "$PID"
    else
        kill "$PID"
    fi

    # Wait for process to terminate (up to 10 minutes)
    timeout=600
    while kill -0 "$PID" 2>/dev/null; do
        sleep 1
        timeout=$((timeout - 1))
        if [ $timeout -eq 0 ]; then
            echo "bitcoind (BCH) did not stop gracefully. Force killing..."
            kill -9 "$PID"
        fi
    done
    rm -f "$PIDFILE"
    echo "bitcoind (BCH) stopped."
}

status() {
    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "bitcoind (BCH) is running (PID $PID)."
        else
            echo "bitcoind (BCH) PID file found but process is not running. Cleaning up."
            rm -f "$PIDFILE"
            exit 1
        fi
    else
        echo "bitcoind (BCH) is not running."
        exit 1
    fi
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    status)  status ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac

exit 0
