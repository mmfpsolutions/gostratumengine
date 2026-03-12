#!/bin/bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#
# service.sh — Start/stop/restart/status for DigiByte node.
# Usage: ./service.sh {start|stop|restart|status}

# Configuration
DATADIR="{BASE_PATH}/dgb/data"
PIDFILE="$DATADIR/digibyted.pid"
DAEMON="{BASE_PATH}/dgb/digibyted"
CLI="{BASE_PATH}/dgb/digibyte-cli"

if [ ! -d "$DATADIR" ]; then
    echo "Error: Data directory $DATADIR does not exist."
    echo "Please create it and ensure proper permissions before running this script."
    exit 1
fi

start() {
    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "digibyted is already running (PID $PID)."
            exit 1
        else
            echo "Stale PID file found. Cleaning up."
            rm -f "$PIDFILE"
        fi
    fi

    echo "Starting digibyted..."
    $DAEMON -datadir="$DATADIR" -printtoconsole=0 &
    PID=$!
    echo $PID > "$PIDFILE"
    echo "digibyted started (PID $PID)."
}

stop() {
    if [ ! -f "$PIDFILE" ]; then
        echo "digibyted is not running (no PID file)."
        exit 1
    fi

    PID=$(cat "$PIDFILE")
    echo "Stopping digibyted (PID $PID)..."

    # Use digibyte-cli stop for graceful shutdown if available
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
            echo "digibyted did not stop gracefully. Force killing..."
            kill -9 "$PID"
        fi
    done
    rm -f "$PIDFILE"
    echo "digibyted stopped."
}

status() {
    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "digibyted is running (PID $PID)."
        else
            echo "digibyted PID file found but process is not running. Cleaning up."
            rm -f "$PIDFILE"
            exit 1
        fi
    else
        echo "digibyted is not running."
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
