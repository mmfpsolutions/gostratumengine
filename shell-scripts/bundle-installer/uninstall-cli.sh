#!/usr/bin/env bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#
# uninstall-cli.sh — Interactive uninstaller for GoStratumEngine bundle.
# Reads the manifest written by install-cli.sh to know what to remove.
#
# Usage:
#   sudo bash -c "$(curl -sSL https://get.gostratumengine.io/scripts/uninstall-cli.sh)"
#
set -euo pipefail

# ─── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()    { echo -e "${CYAN}[INFO]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET} $*"; }
error()   { echo -e "${RED}[ERROR]${RESET} $*" >&2; }
success() { echo -e "${GREEN}[OK]${RESET} $*"; }

# ─── Root Check ───────────────────────────────────────────────────────────────

if [[ $EUID -ne 0 ]]; then
    error "This uninstaller must be run as root."
    error "Usage: sudo bash -c \"\$(curl -sSL https://get.gostratumengine.io/scripts/uninstall-cli.sh)\""
    exit 1
fi

# ─── Locate Manifest ─────────────────────────────────────────────────────────

echo
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║       GoStratumEngine Bundle Uninstaller                ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${RESET}"
echo

BASE_PATH=""
read -rp "$(echo -e "${BOLD}Base install path${RESET} [/mining]: ")" BASE_PATH
BASE_PATH="${BASE_PATH:-/mining}"

MANIFEST="${BASE_PATH}/.gse-manifest.json"

if [[ ! -f "$MANIFEST" ]]; then
    error "No installation manifest found at ${MANIFEST}"
    error "Cannot proceed without a manifest. Was GoStratumEngine installed with install-cli.sh?"
    exit 1
fi

info "Found manifest at ${MANIFEST}"

# ─── Parse Manifest ──────────────────────────────────────────────────────────

# Helper to extract JSON values (simple grep-based, no jq needed)
json_val() {
    local key="$1"
    grep -o "\"${key}\": *\"[^\"]*\"" "$MANIFEST" | head -1 | sed 's/.*: *"//;s/"$//'
}

json_bool() {
    local key="$1"
    grep -o "\"${key}\": *[a-z]*" "$MANIFEST" | head -1 | sed 's/.*: *//'
}

RUN_USER=$(json_val "run_user")
NODE_INSTALLED=$(json_bool "installed" | head -1)  # first match is gse
# More specific parsing for nested values
NODE_INSTALLED=$(grep -A2 '"node"' "$MANIFEST" | grep '"installed"' | head -1 | sed 's/.*: *//;s/,$//')
NODE_COIN=$(json_val "coin")
NODE_COIN_LOWER=$(json_val "coin_lower")
NODE_DATA_PATH=$(json_val "data_path")
NODE_DAEMON=$(json_val "daemon")
NODE_CLI=$(json_val "cli")

WEBUI_INSTALLED=$(grep -A2 '"webui"' "$MANIFEST" | grep '"installed"' | head -1 | sed 's/.*: *//;s/,$//')

LOGROTATION_INSTALLED=$(grep -A2 '"log_rotation"' "$MANIFEST" | grep '"installed"' | head -1 | sed 's/.*: *//;s/,$//')
CRONTAB_MARKER=$(json_val "crontab_marker")

SYSTEMD_INSTALLED=$(grep -A2 '"systemd"' "$MANIFEST" | grep '"installed"' | head -1 | sed 's/.*: *//;s/,$//')
# Extract service names from the services array
SYSTEMD_SERVICES=()
while IFS= read -r svc; do
    [[ -n "$svc" ]] && SYSTEMD_SERVICES+=("$svc")
done < <(grep -o '"[a-z-]*\.service"' "$MANIFEST" | tr -d '"')

# ─── Display What Will Be Removed ────────────────────────────────────────────

echo
echo -e "${BOLD}${RED}WARNING: This will remove the following components:${RESET}"
echo
echo -e "  - GoStratumEngine from ${BASE_PATH}/gse/"
if [[ "$NODE_INSTALLED" == "true" ]]; then
    echo -e "  - ${NODE_COIN} Node from ${BASE_PATH}/${NODE_COIN_LOWER}/"
fi
if [[ "$WEBUI_INSTALLED" == "true" ]]; then
    echo -e "  - Web Dashboard from ${BASE_PATH}/gse-webui/"
fi
if [[ "$LOGROTATION_INSTALLED" == "true" ]]; then
    echo -e "  - Log rotation config from ${BASE_PATH}/log_rotation/"
    echo -e "  - Crontab entry: ${CRONTAB_MARKER}"
fi
if [[ "$SYSTEMD_INSTALLED" == "true" && ${#SYSTEMD_SERVICES[@]} -gt 0 ]]; then
    echo -e "  - Systemd services: ${SYSTEMD_SERVICES[*]}"
fi

if [[ "$NODE_INSTALLED" == "true" ]]; then
    echo
    echo -e "  ${BOLD}${RED}CRITICAL:${RESET} The node data directory ${NODE_DATA_PATH}/"
    echo -e "  contains blockchain data. Removing it will require a full"
    echo -e "  re-sync which can take days or weeks."
fi

echo
echo -e "${BOLD}Choose an option:${RESET}"
echo "  [1] Remove everything (including blockchain data)"
echo "  [2] Remove software only (keep blockchain data)"
echo "  [3] Cancel"
echo

REMOVE_MODE=""
while true; do
    read -rp "$(echo -e "${BOLD}Select [1-3]:${RESET} ")" choice
    case "$choice" in
        1) REMOVE_MODE="full"; break ;;
        2) REMOVE_MODE="software"; break ;;
        3) info "Cancelled."; exit 0 ;;
        *) warn "Please enter 1, 2, or 3." ;;
    esac
done

echo
echo -e "${BOLD}${RED}Type CONFIRM to proceed with uninstallation:${RESET}"
read -rp "> " confirmation
if [[ "$confirmation" != "CONFIRM" ]]; then
    info "Uninstallation cancelled."
    exit 0
fi

echo

# ─── Stop Running Services ───────────────────────────────────────────────────

info "Stopping services..."

# Stop via systemd if available
if [[ "$SYSTEMD_INSTALLED" == "true" ]]; then
    for svc in "${SYSTEMD_SERVICES[@]}"; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            systemctl stop "$svc"
            success "Stopped ${svc}"
        fi
    done
fi

# Also stop via PID files in case systemd wasn't used
# Stop the node first (uses cli for graceful shutdown, falls back to kill)
if [[ "$NODE_INSTALLED" == "true" ]]; then
    NODE_PID_FILE="${NODE_DATA_PATH}/${NODE_DAEMON}.pid"
    # Try cli stop first for graceful shutdown
    if [[ -x "${BASE_PATH}/${NODE_COIN_LOWER}/${NODE_CLI}" ]]; then
        "${BASE_PATH}/${NODE_COIN_LOWER}/${NODE_CLI}" -datadir="${NODE_DATA_PATH}" stop 2>/dev/null && \
            success "Sent stop command to ${NODE_COIN} node" || true
    fi
    # Fall back to PID file
    if [[ -f "$NODE_PID_FILE" ]]; then
        local_pid=$(cat "$NODE_PID_FILE")
        if kill -0 "$local_pid" 2>/dev/null; then
            kill "$local_pid"
            success "Stopped ${NODE_COIN} node (PID ${local_pid})"
        fi
    else
        # No PID file — try to find the running process
        local_pid=$(pgrep -f "${NODE_DAEMON}.*${NODE_DATA_PATH}" 2>/dev/null || true)
        if [[ -n "$local_pid" ]]; then
            kill "$local_pid"
            success "Stopped ${NODE_COIN} node (PID ${local_pid})"
        fi
    fi
    # Wait for node to actually stop before removing files
    if [[ -n "${local_pid:-}" ]] && kill -0 "$local_pid" 2>/dev/null; then
        info "Waiting for ${NODE_COIN} node to shut down..."
        timeout=60
        while kill -0 "$local_pid" 2>/dev/null; do
            sleep 1
            timeout=$((timeout - 1))
            if [[ $timeout -eq 0 ]]; then
                warn "Node did not stop gracefully. Force killing..."
                kill -9 "$local_pid" 2>/dev/null || true
                break
            fi
        done
    fi
fi

if [[ -f "${BASE_PATH}/gse/gse.pid" ]]; then
    local_pid=$(cat "${BASE_PATH}/gse/gse.pid")
    if kill -0 "$local_pid" 2>/dev/null; then
        kill "$local_pid"
        success "Stopped GSE (PID ${local_pid})"
    fi
    rm -f "${BASE_PATH}/gse/gse.pid"
fi

if [[ -f "${BASE_PATH}/gse-webui/dashboard.pid" ]]; then
    local_pid=$(cat "${BASE_PATH}/gse-webui/dashboard.pid")
    if kill -0 "$local_pid" 2>/dev/null; then
        kill "$local_pid"
        success "Stopped web dashboard (PID ${local_pid})"
    fi
    rm -f "${BASE_PATH}/gse-webui/dashboard.pid"
fi

# ─── Remove Systemd Services ─────────────────────────────────────────────────

if [[ "$SYSTEMD_INSTALLED" == "true" ]]; then
    info "Removing systemd services..."
    for svc in "${SYSTEMD_SERVICES[@]}"; do
        if [[ -f "/etc/systemd/system/${svc}" ]]; then
            systemctl disable "$svc" 2>/dev/null || true
            rm -f "/etc/systemd/system/${svc}"
            success "Removed ${svc}"
        fi
    done
    systemctl daemon-reload
fi

# ─── Remove Crontab Entry ────────────────────────────────────────────────────

if [[ "$LOGROTATION_INSTALLED" == "true" && -n "$CRONTAB_MARKER" ]]; then
    info "Removing crontab entry..."
    if crontab -l 2>/dev/null | grep -q "$CRONTAB_MARKER"; then
        (crontab -l 2>/dev/null | grep -v "$CRONTAB_MARKER" || true) | crontab -
        success "Removed crontab entry."
    else
        info "No crontab entry found."
    fi
fi

# ─── Remove Files ────────────────────────────────────────────────────────────

info "Removing installed files..."

# Always remove GSE
if [[ -d "${BASE_PATH}/gse" ]]; then
    rm -rf "${BASE_PATH}/gse"
    success "Removed ${BASE_PATH}/gse/"
fi

# Web UI
if [[ "$WEBUI_INSTALLED" == "true" && -d "${BASE_PATH}/gse-webui" ]]; then
    rm -rf "${BASE_PATH}/gse-webui"
    success "Removed ${BASE_PATH}/gse-webui/"
fi

# Log rotation
if [[ "$LOGROTATION_INSTALLED" == "true" && -d "${BASE_PATH}/log_rotation" ]]; then
    rm -rf "${BASE_PATH}/log_rotation"
    success "Removed ${BASE_PATH}/log_rotation/"
fi

# Node
if [[ "$NODE_INSTALLED" == "true" ]]; then
    if [[ "$REMOVE_MODE" == "full" ]]; then
        # Remove everything including blockchain data
        if [[ -d "${BASE_PATH}/${NODE_COIN_LOWER}" ]]; then
            rm -rf "${BASE_PATH}/${NODE_COIN_LOWER}"
            success "Removed ${BASE_PATH}/${NODE_COIN_LOWER}/ (including blockchain data)"
        fi
    else
        # Software only — keep data directory, remove binaries and configs
        if [[ -d "${BASE_PATH}/${NODE_COIN_LOWER}" ]]; then
            rm -f "${BASE_PATH}/${NODE_COIN_LOWER}/${NODE_DAEMON}"
            rm -f "${BASE_PATH}/${NODE_COIN_LOWER}/${NODE_CLI}"
            success "Removed ${NODE_COIN} binaries (blockchain data preserved at ${NODE_DATA_PATH}/)"
        fi
    fi
fi

# Remove manifest
rm -f "$MANIFEST"

# Check if base path is now empty
if [[ -d "$BASE_PATH" ]]; then
    if [[ -z "$(ls -A "$BASE_PATH" 2>/dev/null)" ]]; then
        rmdir "$BASE_PATH"
        success "Removed empty directory ${BASE_PATH}/"
    else
        info "Directory ${BASE_PATH}/ is not empty — keeping it."
    fi
fi

# ─── Done ─────────────────────────────────────────────────────────────────────

echo
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║     GoStratumEngine Uninstallation Complete!            ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${RESET}"
echo
if [[ "$REMOVE_MODE" == "software" && "$NODE_INSTALLED" == "true" ]]; then
    echo -e "  Blockchain data preserved at: ${NODE_DATA_PATH}/"
    echo
fi
echo -e "  All GoStratumEngine components have been removed."
echo
