#!/usr/bin/env bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
#
# install-cli.sh — Interactive bundle installer for GoStratumEngine.
# Installs GSE, a crypto node, the web dashboard, log rotation, and systemd services.
#
# Usage:
#   sudo bash -c "$(curl -sSL https://get.gostratumengine.io/scripts/install-cli.sh)"
#
# Testing flags:
#   --repo <org/repo>     Override the GSE GitHub repo (default: mmfpsolutions/gostratumengine)
#   --branch <branch>     Override the branch for template fetching (default: main)
#
set -euo pipefail

# ─── Constants ────────────────────────────────────────────────────────────────

INSTALLER_VERSION="1.0.0"

# GitHub repos (GSE_REPO can be overridden via --repo flag)
GSE_REPO="mmfpsolutions/gostratumengine"
GSE_BRANCH="main"
BTC_REPO="bitcoinknots/bitcoin"
BCH_REPO="bitcoin-cash-node/bitcoin-cash-node"
DGB_REPO="DigiByte-Core/digibyte"

# Parse optional flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo)   GSE_REPO="$2"; shift 2 ;;
        --branch) GSE_BRANCH="$2"; shift 2 ;;
        *)        shift ;;
    esac
done

# Raw content base URL for fetching templates and scripts
RAW_BASE="https://raw.githubusercontent.com/${GSE_REPO}/${GSE_BRANCH}"

# ─── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# ─── Utility Functions ────────────────────────────────────────────────────────

info()    { echo -e "${CYAN}[INFO]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET} $*"; }
error()   { echo -e "${RED}[ERROR]${RESET} $*" >&2; }
success() { echo -e "${GREEN}[OK]${RESET} $*"; }

prompt_input() {
    local prompt="$1" default="$2" var_name="$3"
    local input
    read -rp "$(echo -e "${BOLD}${prompt}${RESET} [${default}]: ")" input
    eval "${var_name}=\"${input:-$default}\""
}

prompt_secret() {
    local prompt="$1" default="$2" var_name="$3"
    local input
    read -rsp "$(echo -e "${BOLD}${prompt}${RESET} [${default:+auto-generate}]: ")" input
    echo
    if [[ -z "$input" && -z "$default" ]]; then
        input=$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 32)
        info "Auto-generated password."
    fi
    eval "${var_name}=\"${input:-$default}\""
}

confirm() {
    local prompt="$1"
    local reply
    read -rp "$(echo -e "${BOLD}${prompt}${RESET} [y/N]: ")" reply
    [[ "$reply" =~ ^[Yy]$ ]]
}

# Download with retry (up to 3 attempts)
download() {
    local url="$1" dest="$2" description="$3"
    local attempt
    for attempt in 1 2 3; do
        if curl -sSfL -o "$dest" "$url" 2>/dev/null; then
            return 0
        fi
        if [[ $attempt -lt 3 ]]; then
            warn "Download failed (attempt ${attempt}/3): ${description}. Retrying in 5s..."
            sleep 5
        fi
    done
    error "Failed to download ${description} after 3 attempts."
    error "URL: ${url}"
    return 1
}

# ─── Cleanup Trap ─────────────────────────────────────────────────────────────

CLEANUP_DIRS=()
TMPDIR_CLEANUP=""
MANIFEST_WRITTEN=false

cleanup() {
    if [[ "$MANIFEST_WRITTEN" == true ]]; then
        # Install completed — only clean up temp dir
        [[ -n "$TMPDIR_CLEANUP" ]] && rm -rf "$TMPDIR_CLEANUP"
        return
    fi
    # Partial install — remove everything we created
    if [[ ${#CLEANUP_DIRS[@]} -gt 0 ]]; then
        warn "Cleaning up partial installation..."
        for dir in "${CLEANUP_DIRS[@]}"; do
            [[ -d "$dir" ]] && rm -rf "$dir"
        done
    fi
    [[ -n "$TMPDIR_CLEANUP" ]] && rm -rf "$TMPDIR_CLEANUP"
}

trap cleanup EXIT

# ─── Prerequisite Checks ─────────────────────────────────────────────────────

check_prerequisites() {
    # Must run as root
    if [[ $EUID -ne 0 ]]; then
        error "This installer must be run as root."
        error "Usage: sudo bash -c \"\$(curl -sSL https://get.gostratumengine.io/scripts/install-cli.sh)\""
        exit 1
    fi

    RUN_USER="${SUDO_USER:-$(whoami)}"
    RUN_GROUP=$(id -gn "$RUN_USER" 2>/dev/null || echo "$RUN_USER")

    # Required tools
    local missing=()
    for cmd in curl openssl tar xxd; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        error "Missing required tools: ${missing[*]}"
        error "Install them and try again."
        exit 1
    fi

    # Detect OS and architecture
    OS=$(uname -s)
    ARCH=$(uname -m)

    case "$OS" in
        Linux)  OS_LOWER="linux" ;;
        Darwin) OS_LOWER="darwin" ;;
        *)      error "Unsupported OS: $OS"; exit 1 ;;
    esac

    case "$ARCH" in
        x86_64)       ARCH_LOWER="amd64"; GNU_ARCH="x86_64" ;;
        aarch64|arm64) ARCH_LOWER="arm64"; GNU_ARCH="aarch64" ;;
        *)            error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    PLATFORM="${OS_LOWER}-${ARCH_LOWER}"
}

# ─── Interactive Prompts ──────────────────────────────────────────────────────

collect_user_input() {
    echo
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${BOLD}║       GoStratumEngine Bundle Installer v${INSTALLER_VERSION}            ║${RESET}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${RESET}"
    echo

    # Step 1: Component selection
    echo -e "${BOLD}Which components would you like to install?${RESET}"
    echo "  [1] GSE only"
    echo "  [2] GSE + Crypto Node"
    echo "  [3] GSE + Crypto Node + Web Dashboard"
    echo "  [4] Everything (GSE + Node + Dashboard + Log Rotation + Services)"
    echo
    local choice
    while true; do
        read -rp "$(echo -e "${BOLD}Select [1-4]:${RESET} ")" choice
        case "$choice" in
            1) INSTALL_NODE=false; INSTALL_WEBUI=false; INSTALL_LOGROTATION=false; INSTALL_SYSTEMD=false; break ;;
            2) INSTALL_NODE=true;  INSTALL_WEBUI=false; INSTALL_LOGROTATION=false; INSTALL_SYSTEMD=false; break ;;
            3) INSTALL_NODE=true;  INSTALL_WEBUI=true;  INSTALL_LOGROTATION=false; INSTALL_SYSTEMD=false; break ;;
            4) INSTALL_NODE=true;  INSTALL_WEBUI=true;  INSTALL_LOGROTATION=true;  INSTALL_SYSTEMD=true;  break ;;
            *) warn "Please enter 1, 2, 3, or 4." ;;
        esac
    done

    # Step 2: Coin selection (if node)
    COIN=""
    COIN_LOWER=""
    COIN_TYPE=""
    NODE_REPO=""
    DAEMON_NAME=""
    CLI_NAME=""
    NODE_CONF_NAME=""
    RPC_PORT=""

    if [[ "$INSTALL_NODE" == true ]]; then
        echo
        echo -e "${BOLD}Which crypto node would you like to install?${RESET}"
        echo "  [1] BTC (Bitcoin Knots)"
        echo "  [2] BCH (Bitcoin Cash Node)"
        echo "  [3] DGB (DigiByte)"
        echo
        while true; do
            read -rp "$(echo -e "${BOLD}Select [1-3]:${RESET} ")" choice
            case "$choice" in
                1)
                    COIN="BTC"; COIN_LOWER="btc"; COIN_TYPE="bitcoin"
                    NODE_REPO="$BTC_REPO"; DAEMON_NAME="bitcoind"; CLI_NAME="bitcoin-cli"
                    NODE_CONF_NAME="bitcoin.conf"; RPC_PORT=8332
                    break ;;
                2)
                    COIN="BCH"; COIN_LOWER="bch"; COIN_TYPE="bitcoincash"
                    NODE_REPO="$BCH_REPO"; DAEMON_NAME="bitcoind"; CLI_NAME="bitcoin-cli"
                    NODE_CONF_NAME="bitcoin.conf"; RPC_PORT=8332
                    break ;;
                3)
                    COIN="DGB"; COIN_LOWER="dgb"; COIN_TYPE="digibyte"
                    NODE_REPO="$DGB_REPO"; DAEMON_NAME="digibyted"; CLI_NAME="digibyte-cli"
                    NODE_CONF_NAME="digibyte.conf"; RPC_PORT=14022
                    break ;;
                *) warn "Please enter 1, 2, or 3." ;;
            esac
        done
    fi

    # Step 3: Base path
    echo
    prompt_input "Base install path" "/mining" BASE_PATH

    # Step 4: RPC credentials
    if [[ "$INSTALL_NODE" == true ]]; then
        echo
        prompt_input "RPC username" "gseuser" RPC_USER
        prompt_secret "RPC password (blank to auto-generate)" "" RPC_PASS
    else
        # GSE-only still needs RPC creds for config
        echo
        info "GSE needs RPC credentials to connect to your existing node."
        prompt_input "RPC username" "gseuser" RPC_USER
        prompt_secret "RPC password (blank to auto-generate)" "" RPC_PASS
    fi

    # Step 5: Pruning (if node)
    PRUNE_SETTING=""
    if [[ "$INSTALL_NODE" == true ]]; then
        echo
        echo -e "${BOLD}Enable blockchain pruning?${RESET}"
        echo "  [1] Yes — 10GB  (prune=10000)"
        echo "  [2] Yes — 50GB  (prune=50000)"
        echo "  [3] No  — full node"
        echo
        while true; do
            read -rp "$(echo -e "${BOLD}Select [1-3]:${RESET} ")" choice
            case "$choice" in
                1) PRUNE_SETTING="prune=10000"; break ;;
                2) PRUNE_SETTING="prune=50000"; break ;;
                3) PRUNE_SETTING="# Pruning disabled — full node"; break ;;
                *) warn "Please enter 1, 2, or 3." ;;
            esac
        done
    fi

    # Step 6: Architecture confirmation
    echo
    echo -e "${BOLD}Detected platform:${RESET} ${PLATFORM}"
    if ! confirm "Is this correct?"; then
        echo
        echo "  [1] linux-amd64"
        echo "  [2] linux-arm64"
        echo "  [3] darwin-amd64"
        echo "  [4] darwin-arm64"
        echo
        while true; do
            read -rp "$(echo -e "${BOLD}Select [1-4]:${RESET} ")" choice
            case "$choice" in
                1) PLATFORM="linux-amd64"; OS_LOWER="linux"; ARCH_LOWER="amd64"; GNU_ARCH="x86_64"; break ;;
                2) PLATFORM="linux-arm64"; OS_LOWER="linux"; ARCH_LOWER="arm64"; GNU_ARCH="aarch64"; break ;;
                3) PLATFORM="darwin-amd64"; OS_LOWER="darwin"; ARCH_LOWER="amd64"; GNU_ARCH="x86_64"; break ;;
                4) PLATFORM="darwin-arm64"; OS_LOWER="darwin"; ARCH_LOWER="arm64"; GNU_ARCH="arm64"; break ;;
                *) warn "Please enter 1, 2, 3, or 4." ;;
            esac
        done
    fi

    # Step 7: Optional features (unless already set by option 4)
    if [[ "$INSTALL_LOGROTATION" == false ]]; then
        echo
        if confirm "Configure automatic log rotation?"; then
            INSTALL_LOGROTATION=true
        fi
    fi

    if [[ "$INSTALL_SYSTEMD" == false && "$OS_LOWER" == "linux" ]]; then
        echo
        if confirm "Register systemd services for auto-start on boot?"; then
            INSTALL_SYSTEMD=true
        fi
    elif [[ "$OS_LOWER" == "darwin" ]]; then
        INSTALL_SYSTEMD=false
    fi

    # Python3 check for webui
    if [[ "$INSTALL_WEBUI" == true ]]; then
        if ! command -v python3 &>/dev/null; then
            warn "python3 not found. Web dashboard requires Python 3.6+."
            if ! confirm "Continue without the web dashboard?"; then
                error "Please install python3 and try again."
                exit 1
            fi
            INSTALL_WEBUI=false
        fi
    fi
}

# ─── Confirmation Summary ────────────────────────────────────────────────────

show_summary() {
    echo
    echo -e "${BOLD}═══════════════════════════════════════════════════════════${RESET}"
    echo -e "${BOLD}  Installation Summary${RESET}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════════${RESET}"
    echo
    echo -e "  Platform:        ${CYAN}${PLATFORM}${RESET}"
    echo -e "  Base path:       ${CYAN}${BASE_PATH}${RESET}"
    echo -e "  Run-as user:     ${CYAN}${RUN_USER}${RESET}"
    echo

    echo -e "  ${BOLD}Components:${RESET}"
    echo -e "    GoStratumEngine:  ${GREEN}yes${RESET}  → ${BASE_PATH}/gse/"
    if [[ "$INSTALL_NODE" == true ]]; then
        echo -e "    ${COIN} Node:          ${GREEN}yes${RESET}  → ${BASE_PATH}/${COIN_LOWER}/"
    else
        echo -e "    Crypto Node:      ${YELLOW}no${RESET}"
    fi
    if [[ "$INSTALL_WEBUI" == true ]]; then
        echo -e "    Web Dashboard:    ${GREEN}yes${RESET}  → ${BASE_PATH}/gse-webui/"
    else
        echo -e "    Web Dashboard:    ${YELLOW}no${RESET}"
    fi
    if [[ "$INSTALL_LOGROTATION" == true ]]; then
        echo -e "    Log Rotation:     ${GREEN}yes${RESET}  → ${BASE_PATH}/log_rotation/"
    else
        echo -e "    Log Rotation:     ${YELLOW}no${RESET}"
    fi
    if [[ "$INSTALL_SYSTEMD" == true ]]; then
        echo -e "    Systemd Services: ${GREEN}yes${RESET}"
    else
        echo -e "    Systemd Services: ${YELLOW}no${RESET}"
    fi

    if [[ "$INSTALL_NODE" == true ]]; then
        echo
        echo -e "  ${BOLD}Node Configuration:${RESET}"
        echo -e "    RPC User:      ${RPC_USER}"
        echo -e "    RPC Password:  ******* (will be shown at the end)"
        echo -e "    Pruning:       ${PRUNE_SETTING}"
    fi

    echo
    echo -e "${BOLD}═══════════════════════════════════════════════════════════${RESET}"
    echo

    # Check for existing install
    if [[ -f "${BASE_PATH}/.gse-manifest.json" ]]; then
        warn "An existing installation was found at ${BASE_PATH}."
        echo "  [1] Overwrite (existing configs will be backed up)"
        echo "  [2] Abort"
        echo
        while true; do
            read -rp "$(echo -e "${BOLD}Select [1-2]:${RESET} ")" choice
            case "$choice" in
                1) info "Will back up existing configs before overwriting."; break ;;
                2) info "Aborted."; exit 0 ;;
                *) warn "Please enter 1 or 2." ;;
            esac
        done
        echo
    fi

    if ! confirm "Proceed with installation?"; then
        info "Aborted."
        exit 0
    fi
}

# ─── Template Fetching ────────────────────────────────────────────────────────

fetch_templates() {
    info "Fetching templates from GitHub..."
    TMPDIR_CLEANUP=$(mktemp -d)

    # Always fetch main GSE config template
    download "${RAW_BASE}/templates/gse/config.json.template" \
        "${TMPDIR_CLEANUP}/config.json.template" \
        "GSE config template"

    # Fetch coin-specific templates if node selected
    if [[ "$INSTALL_NODE" == true ]]; then
        local coin_dir="$COIN_LOWER"
        local conf_template_name="${NODE_CONF_NAME}.template"

        # GSE coin config template
        download "${RAW_BASE}/templates/coins/${coin_dir}/gse-config.json.template" \
            "${TMPDIR_CLEANUP}/gse-coin-config.template" \
            "${COIN} GSE config template"

        # Node config template
        # DGB uses "digitbyte.conf.template" in the repo
        local remote_conf_name="${NODE_CONF_NAME}.template"
        if [[ "$COIN_LOWER" == "dgb" ]]; then
            remote_conf_name="digitbyte.conf.template"
        fi
        download "${RAW_BASE}/templates/coins/${coin_dir}/${remote_conf_name}" \
            "${TMPDIR_CLEANUP}/node-config.template" \
            "${COIN} node config template"

        # Node service script
        download "${RAW_BASE}/templates/coins/${coin_dir}/service.sh" \
            "${TMPDIR_CLEANUP}/node-service.sh" \
            "${COIN} node service script"
    fi

    # Fetch management scripts
    download "${RAW_BASE}/shell-scripts/gostratumengine/gse.sh" \
        "${TMPDIR_CLEANUP}/gse.sh" \
        "GSE management script"

    if [[ "$INSTALL_WEBUI" == true ]]; then
        download "${RAW_BASE}/simple-web-ui/server.py" \
            "${TMPDIR_CLEANUP}/server.py" \
            "Web dashboard server"
        download "${RAW_BASE}/simple-web-ui/dashboard.sh" \
            "${TMPDIR_CLEANUP}/dashboard.sh" \
            "Web dashboard management script"
    fi

    if [[ "$INSTALL_LOGROTATION" == true ]]; then
        download "${RAW_BASE}/shell-scripts/log%20rotations/log_rotation.sh" \
            "${TMPDIR_CLEANUP}/log_rotation.sh" \
            "Log rotation script"
    fi

    success "Templates fetched."
}

# ─── Directory Creation ───────────────────────────────────────────────────────

create_directories() {
    info "Creating directories..."

    mkdir -p "${BASE_PATH}/gse/logs"
    CLEANUP_DIRS+=("${BASE_PATH}/gse")

    if [[ "$INSTALL_NODE" == true ]]; then
        mkdir -p "${BASE_PATH}/${COIN_LOWER}/data"
        CLEANUP_DIRS+=("${BASE_PATH}/${COIN_LOWER}")
    fi

    if [[ "$INSTALL_WEBUI" == true ]]; then
        mkdir -p "${BASE_PATH}/gse-webui/logs"
        CLEANUP_DIRS+=("${BASE_PATH}/gse-webui")
    fi

    if [[ "$INSTALL_LOGROTATION" == true ]]; then
        mkdir -p "${BASE_PATH}/log_rotation"
        CLEANUP_DIRS+=("${BASE_PATH}/log_rotation")
    fi

    # Set ownership to the run-as user
    chown -R "${RUN_USER}:${RUN_GROUP}" "${BASE_PATH}"
    chmod 750 "${BASE_PATH}"

    success "Directories created."
}

# ─── RPC Auth Generation ─────────────────────────────────────────────────────

generate_rpcauth() {
    local user="$1" pass="$2"
    local salt hash
    salt=$(openssl rand -hex 16)
    hash=$(echo -n "${pass}" | openssl dgst -sha256 -hmac "${salt}" -binary | xxd -p -c 256)
    echo "${user}:${salt}\$${hash}"
}

# ─── Component Installation ──────────────────────────────────────────────────

install_gse() {
    info "Installing GoStratumEngine..."

    # Get latest release tag
    local release_json tag version download_url
    release_json=$(curl -sSf "https://api.github.com/repos/${GSE_REPO}/releases/latest" 2>/dev/null) || {
        error "Failed to query GitHub API for GSE releases."
        error "You may be rate-limited. Try again later or download manually."
        return 1
    }

    tag=$(echo "$release_json" | grep -o '"tag_name": *"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')
    version="${tag#v}"

    download_url="https://github.com/${GSE_REPO}/releases/download/${tag}/gostratumengine-${PLATFORM}"
    download "$download_url" "${BASE_PATH}/gse/gostratumengine" "GSE binary (${version})"
    chmod +x "${BASE_PATH}/gse/gostratumengine"

    # Copy management script
    cp "${TMPDIR_CLEANUP}/gse.sh" "${BASE_PATH}/gse/gse.sh"
    chmod +x "${BASE_PATH}/gse/gse.sh"

    GSE_VERSION="$version"
    success "GSE ${version} installed."
}

install_node() {
    [[ "$INSTALL_NODE" != true ]] && return

    info "Installing ${COIN} node..."

    local release_json tag version
    release_json=$(curl -sSf "https://api.github.com/repos/${NODE_REPO}/releases/latest" 2>/dev/null) || {
        error "Failed to query GitHub API for ${COIN} releases."
        return 1
    }

    tag=$(echo "$release_json" | grep -o '"tag_name": *"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')
    version="${tag#v}"

    # Build the asset filename based on coin
    local asset_name arch_suffix download_url
    case "$COIN_LOWER" in
        btc)
            # Bitcoin Knots: bitcoin-knots-VERSION-ARCH.tar.gz
            if [[ "$OS_LOWER" == "darwin" && "$ARCH_LOWER" == "arm64" ]]; then
                arch_suffix="arm64-apple-darwin"
            elif [[ "$OS_LOWER" == "darwin" ]]; then
                arch_suffix="x86_64-apple-darwin"
            else
                arch_suffix="${GNU_ARCH}-linux-gnu"
            fi
            asset_name="bitcoin-knots-${version}-${arch_suffix}.tar.gz"
            ;;
        bch)
            # Bitcoin Cash Node: bitcoin-cash-node-VERSION-ARCH.tar.gz
            if [[ "$OS_LOWER" == "darwin" ]]; then
                arch_suffix="osx64"
            else
                arch_suffix="${GNU_ARCH}-linux-gnu"
            fi
            asset_name="bitcoin-cash-node-${version}-${arch_suffix}.tar.gz"
            ;;
        dgb)
            # DigiByte: digibyte-VERSION-ARCH.tar.gz
            if [[ "$OS_LOWER" == "darwin" && "$ARCH_LOWER" == "arm64" ]]; then
                arch_suffix="arm64-apple-darwin"
            elif [[ "$OS_LOWER" == "darwin" ]]; then
                arch_suffix="x86_64-apple-darwin"
            else
                arch_suffix="${GNU_ARCH}-linux-gnu"
            fi
            asset_name="digibyte-${version}-${arch_suffix}.tar.gz"
            ;;
    esac

    download_url="https://github.com/${NODE_REPO}/releases/download/${tag}/${asset_name}"
    local tarball="${TMPDIR_CLEANUP}/${asset_name}"

    download "$download_url" "$tarball" "${COIN} node binary (${version})"

    # Extract and find binaries
    info "Extracting ${COIN} node..."
    local extract_dir="${TMPDIR_CLEANUP}/node-extract"
    mkdir -p "$extract_dir"
    tar -xzf "$tarball" -C "$extract_dir"

    # Find daemon and cli binaries (they're usually in a bin/ subdirectory)
    local daemon_path cli_path
    daemon_path=$(find "$extract_dir" -name "$DAEMON_NAME" -type f | head -1)
    cli_path=$(find "$extract_dir" -name "$CLI_NAME" -type f | head -1)

    if [[ -z "$daemon_path" ]]; then
        error "Could not find ${DAEMON_NAME} in the downloaded archive."
        error "You may need to download and extract manually from:"
        error "  https://github.com/${NODE_REPO}/releases"
        return 1
    fi

    cp "$daemon_path" "${BASE_PATH}/${COIN_LOWER}/${DAEMON_NAME}"
    chmod +x "${BASE_PATH}/${COIN_LOWER}/${DAEMON_NAME}"

    if [[ -n "$cli_path" ]]; then
        cp "$cli_path" "${BASE_PATH}/${COIN_LOWER}/${CLI_NAME}"
        chmod +x "${BASE_PATH}/${COIN_LOWER}/${CLI_NAME}"
    fi

    # Deploy node service script with BASE_PATH substituted
    sed "s|{BASE_PATH}|${BASE_PATH}|g" "${TMPDIR_CLEANUP}/node-service.sh" \
        > "${BASE_PATH}/${COIN_LOWER}/service.sh"
    chmod +x "${BASE_PATH}/${COIN_LOWER}/service.sh"

    NODE_VERSION="$version"
    success "${COIN} node ${version} installed."
}

install_webui() {
    [[ "$INSTALL_WEBUI" != true ]] && return

    info "Installing web dashboard..."

    cp "${TMPDIR_CLEANUP}/server.py" "${BASE_PATH}/gse-webui/server.py"
    cp "${TMPDIR_CLEANUP}/dashboard.sh" "${BASE_PATH}/gse-webui/dashboard.sh"
    chmod +x "${BASE_PATH}/gse-webui/dashboard.sh"

    success "Web dashboard installed."
}

# ─── Configuration Generation ─────────────────────────────────────────────────

generate_configs() {
    info "Generating configuration files..."

    # Back up existing configs if overwriting
    local timestamp
    timestamp=$(date +%Y%m%d%H%M%S)
    if [[ -f "${BASE_PATH}/gse/config.json" ]]; then
        cp "${BASE_PATH}/gse/config.json" "${BASE_PATH}/gse/config.json.bak.${timestamp}"
        info "Backed up existing GSE config."
    fi

    # Generate GSE config.json
    local coin_config main_config
    if [[ "$INSTALL_NODE" == true ]]; then
        # Substitute RPC credentials in coin template
        coin_config=$(cat "${TMPDIR_CLEANUP}/gse-coin-config.template")
        coin_config=$(echo "$coin_config" | sed "s/{RPC_USER}/${RPC_USER}/g")
        coin_config=$(echo "$coin_config" | sed "s/{RPC_PASSWORD}/${RPC_PASS}/g")
    else
        # GSE-only: use a minimal placeholder — user must configure manually
        coin_config=""
    fi

    # Assemble main config
    main_config=$(cat "${TMPDIR_CLEANUP}/config.json.template")
    if [[ -n "$coin_config" ]]; then
        # Replace __COIN_CONFIG__ with the coin config block
        # Use awk to handle multi-line replacement
        echo "$main_config" | awk -v replacement="$coin_config" '{
            if ($0 ~ /__COIN_CONFIG__/) {
                print replacement
            } else {
                print
            }
        }' > "${BASE_PATH}/gse/config.json"
    else
        # No coin — write template as-is with empty coins block
        echo "$main_config" | sed 's/__COIN_CONFIG__//' > "${BASE_PATH}/gse/config.json"
    fi

    chmod 600 "${BASE_PATH}/gse/config.json"

    # Generate node config if installing node
    if [[ "$INSTALL_NODE" == true ]]; then
        if [[ -f "${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}" ]]; then
            cp "${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}" \
               "${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}.bak.${timestamp}"
            info "Backed up existing node config."
        fi

        local rpcauth_string
        rpcauth_string=$(generate_rpcauth "$RPC_USER" "$RPC_PASS")

        local node_config
        node_config=$(cat "${TMPDIR_CLEANUP}/node-config.template")
        node_config=$(echo "$node_config" | sed "s|{RPC_AUTH_STRING}|${rpcauth_string}|g")
        node_config=$(echo "$node_config" | sed "s|{prune_setting}|${PRUNE_SETTING}|g")

        echo "$node_config" > "${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}"
        chmod 600 "${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}"
    fi

    # Set ownership
    chown -R "${RUN_USER}:${RUN_GROUP}" "${BASE_PATH}"

    success "Configuration files generated."
}

# ─── Log Rotation Setup ──────────────────────────────────────────────────────

setup_log_rotation() {
    [[ "$INSTALL_LOGROTATION" != true ]] && return

    info "Setting up log rotation..."

    # Copy script
    cp "${TMPDIR_CLEANUP}/log_rotation.sh" "${BASE_PATH}/log_rotation/log_rotation.sh"
    chmod +x "${BASE_PATH}/log_rotation/log_rotation.sh"

    # Generate config
    local conf="${BASE_PATH}/log_rotation/log_rotation.conf"
    cat > "$conf" <<LOGEOF
# GoStratumEngine Log Rotation Configuration
# Auto-generated by install-cli.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Format: LOG_PATH | MAX_SIZE_MB | MAX_AGE_DAYS | COMPRESS | ARCHIVE_DIR

${BASE_PATH}/gse/logs/gse.log | 50 | 30 | yes | ${BASE_PATH}/gse/logs/archive
LOGEOF

    if [[ "$INSTALL_NODE" == true ]]; then
        echo "${BASE_PATH}/${COIN_LOWER}/data/debug.log | 100 | 14 | yes | ${BASE_PATH}/${COIN_LOWER}/data/archive" >> "$conf"
    fi

    if [[ "$INSTALL_WEBUI" == true ]]; then
        echo "${BASE_PATH}/gse-webui/logs/dashboard.log | 25 | 30 | yes | ${BASE_PATH}/gse-webui/logs/archive" >> "$conf"
    fi

    # Add crontab entry (as the run user, not root)
    local marker="GSE_LOG_ROTATION"
    if crontab -u "$RUN_USER" -l 2>/dev/null | grep -q "$marker"; then
        warn "Log rotation crontab entry already exists, skipping."
    else
        (crontab -u "$RUN_USER" -l 2>/dev/null || true; echo "0 * * * * ${BASE_PATH}/log_rotation/log_rotation.sh ${BASE_PATH}/log_rotation/log_rotation.conf >> ${BASE_PATH}/log_rotation/cron.log 2>&1 # ${marker}") | crontab -u "$RUN_USER" -
        success "Crontab entry added (hourly log rotation)."
    fi

    chown -R "${RUN_USER}:${RUN_GROUP}" "${BASE_PATH}/log_rotation"
    success "Log rotation configured."
}

# ─── Systemd Service Registration ────────────────────────────────────────────

setup_systemd() {
    [[ "$INSTALL_SYSTEMD" != true ]] && return
    [[ "$OS_LOWER" != "linux" ]] && return

    info "Registering systemd services..."

    local services_created=()

    # GSE service
    local gse_wants=""
    if [[ "$INSTALL_NODE" == true ]]; then
        gse_wants="Wants=${COIN_LOWER}-node.service"
    fi

    cat > /etc/systemd/system/gse.service <<SVCEOF
[Unit]
Description=GoStratumEngine Stratum Server
After=network.target
${gse_wants}

[Service]
Type=simple
ExecStart=${BASE_PATH}/gse/gostratumengine -config ${BASE_PATH}/gse/config.json
WorkingDirectory=${BASE_PATH}/gse
Restart=on-failure
RestartSec=10
StandardOutput=append:${BASE_PATH}/gse/logs/gse.log
StandardError=append:${BASE_PATH}/gse/logs/gse.log
User=${RUN_USER}
Group=${RUN_GROUP}

[Install]
WantedBy=multi-user.target
SVCEOF
    services_created+=("gse.service")

    # Node service
    if [[ "$INSTALL_NODE" == true ]]; then
        cat > "/etc/systemd/system/${COIN_LOWER}-node.service" <<SVCEOF
[Unit]
Description=${COIN} Node
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=${BASE_PATH}/${COIN_LOWER}/service.sh start
ExecStop=${BASE_PATH}/${COIN_LOWER}/service.sh stop
WorkingDirectory=${BASE_PATH}/${COIN_LOWER}
User=${RUN_USER}
Group=${RUN_GROUP}

[Install]
WantedBy=multi-user.target
SVCEOF
        services_created+=("${COIN_LOWER}-node.service")
    fi

    # Web UI service
    if [[ "$INSTALL_WEBUI" == true ]]; then
        cat > /etc/systemd/system/gse-webui.service <<SVCEOF
[Unit]
Description=GoStratumEngine Web Dashboard
After=network.target gse.service

[Service]
Type=simple
ExecStart=$(command -v python3) ${BASE_PATH}/gse-webui/server.py
WorkingDirectory=${BASE_PATH}/gse-webui
Environment=API_BASE=http://127.0.0.1:8080
Environment=LISTEN_PORT=8000
Restart=on-failure
RestartSec=5
User=${RUN_USER}
Group=${RUN_GROUP}

[Install]
WantedBy=multi-user.target
SVCEOF
        services_created+=("gse-webui.service")
    fi

    systemctl daemon-reload
    for svc in "${services_created[@]}"; do
        systemctl enable "$svc" 2>/dev/null
        success "Enabled ${svc}"
    done

    SYSTEMD_SERVICES=("${services_created[@]}")
}

# ─── Write Manifest ──────────────────────────────────────────────────────────

write_manifest() {
    info "Writing installation manifest..."

    local manifest="${BASE_PATH}/.gse-manifest.json"

    # Build JSON manually (no jq dependency)
    cat > "$manifest" <<MANEOF
{
  "installer_version": "${INSTALLER_VERSION}",
  "install_date": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "base_path": "${BASE_PATH}",
  "platform": "${PLATFORM}",
  "run_user": "${RUN_USER}",
  "components": {
    "gse": {
      "installed": true,
      "version": "${GSE_VERSION:-unknown}",
      "path": "${BASE_PATH}/gse"
    },
    "node": {
      "installed": ${INSTALL_NODE},
      "coin": "${COIN:-none}",
      "coin_lower": "${COIN_LOWER:-none}",
      "version": "${NODE_VERSION:-none}",
      "path": "${BASE_PATH}/${COIN_LOWER:-none}",
      "data_path": "${BASE_PATH}/${COIN_LOWER:-none}/data",
      "daemon": "${DAEMON_NAME:-none}",
      "cli": "${CLI_NAME:-none}",
      "conf_name": "${NODE_CONF_NAME:-none}"
    },
    "webui": {
      "installed": ${INSTALL_WEBUI},
      "path": "${BASE_PATH}/gse-webui"
    }
  },
  "log_rotation": {
    "installed": ${INSTALL_LOGROTATION},
    "config_path": "${BASE_PATH}/log_rotation/log_rotation.conf",
    "script_path": "${BASE_PATH}/log_rotation/log_rotation.sh",
    "crontab_marker": "GSE_LOG_ROTATION"
  },
  "systemd": {
    "installed": ${INSTALL_SYSTEMD},
    "services": [$(
      if [[ "$INSTALL_SYSTEMD" == true && ${#SYSTEMD_SERVICES[@]} -gt 0 ]]; then
          printf '"%s"' "${SYSTEMD_SERVICES[0]}"
          for svc in "${SYSTEMD_SERVICES[@]:1}"; do
              printf ', "%s"' "$svc"
          done
      fi
    )]
  }
}
MANEOF

    chmod 600 "$manifest"
    chown "${RUN_USER}:${RUN_GROUP}" "$manifest"
    MANIFEST_WRITTEN=true

    success "Manifest written to ${manifest}"
}

# ─── Print Summary ───────────────────────────────────────────────────────────

print_summary() {
    echo
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${BOLD}║     GoStratumEngine Installation Complete!              ║${RESET}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${RESET}"
    echo
    echo -e "  ${BOLD}GSE Binary:${RESET}       ${BASE_PATH}/gse/gostratumengine"
    echo -e "  ${BOLD}GSE Config:${RESET}       ${BASE_PATH}/gse/config.json"
    echo -e "  ${BOLD}GSE Control:${RESET}      ${BASE_PATH}/gse/gse.sh {start|stop|restart|status}"

    if [[ "$INSTALL_NODE" == true ]]; then
        echo
        echo -e "  ${BOLD}${COIN} Daemon:${RESET}       ${BASE_PATH}/${COIN_LOWER}/${DAEMON_NAME}"
        echo -e "  ${BOLD}${COIN} CLI:${RESET}          ${BASE_PATH}/${COIN_LOWER}/${CLI_NAME}"
        echo -e "  ${BOLD}${COIN} Config:${RESET}       ${BASE_PATH}/${COIN_LOWER}/data/${NODE_CONF_NAME}"
        echo -e "  ${BOLD}${COIN} Data:${RESET}         ${BASE_PATH}/${COIN_LOWER}/data/"
        echo -e "  ${BOLD}${COIN} Control:${RESET}      ${BASE_PATH}/${COIN_LOWER}/service.sh {start|stop|restart|status}"
    fi

    if [[ "$INSTALL_WEBUI" == true ]]; then
        echo
        echo -e "  ${BOLD}Dashboard:${RESET}        ${BASE_PATH}/gse-webui/"
        echo -e "  ${BOLD}Dashboard Ctrl:${RESET}   ${BASE_PATH}/gse-webui/dashboard.sh {start|stop|status}"
    fi

    echo
    echo -e "  ${BOLD}${RED}IMPORTANT — Save These Credentials:${RESET}"
    echo -e "  ${BOLD}RPC Username:${RESET}     ${RPC_USER}"
    echo -e "  ${BOLD}RPC Password:${RESET}     ${RPC_PASS}"
    echo

    echo -e "  ${BOLD}Ports:${RESET}"
    echo -e "    Stratum:      3333   (connect miners to <this-host>:3333)"
    echo -e "    GSE API:      8080   (http://127.0.0.1:8080)"
    if [[ "$INSTALL_WEBUI" == true ]]; then
        echo -e "    Dashboard:    8000   (http://<this-host>:8000)"
    fi

    echo
    if [[ "$INSTALL_SYSTEMD" == true ]]; then
        echo -e "  ${BOLD}Services are registered and will auto-start on boot.${RESET}"
        echo -e "  Start all now with:"
        if [[ "$INSTALL_NODE" == true ]]; then
            echo -e "    sudo systemctl start ${COIN_LOWER}-node.service"
            echo -e "    # Wait for node to sync, then:"
        fi
        echo -e "    sudo systemctl start gse.service"
        if [[ "$INSTALL_WEBUI" == true ]]; then
            echo -e "    sudo systemctl start gse-webui.service"
        fi
    else
        echo -e "  ${BOLD}Quick Start:${RESET}"
        if [[ "$INSTALL_NODE" == true ]]; then
            echo -e "    1. Start the node:"
            echo -e "       ${BASE_PATH}/${COIN_LOWER}/service.sh start"
            echo -e "    2. Wait for sync:"
            echo -e "       ${BASE_PATH}/${COIN_LOWER}/${CLI_NAME} -datadir=${BASE_PATH}/${COIN_LOWER}/data getblockchaininfo"
            echo -e "    3. Start GSE:"
        else
            echo -e "    1. Start GSE:"
        fi
        echo -e "       ${BASE_PATH}/gse/gse.sh start"
        if [[ "$INSTALL_WEBUI" == true ]]; then
            echo -e "    $(( INSTALL_NODE ? 4 : 2 )). Start Dashboard:"
            echo -e "       ${BASE_PATH}/gse-webui/dashboard.sh start"
        fi
    fi

    echo
    echo -e "  ${BOLD}Manifest:${RESET}         ${BASE_PATH}/.gse-manifest.json"
    echo
    echo -e "${BOLD}══════════════════════════════════════════════════════════${RESET}"
    echo
}

# ─── Main ─────────────────────────────────────────────────────────────────────

main() {
    # Initialize state
    GSE_VERSION=""
    NODE_VERSION=""
    SYSTEMD_SERVICES=()

    check_prerequisites
    collect_user_input
    show_summary
    fetch_templates
    create_directories
    install_gse
    install_node
    install_webui
    generate_configs
    setup_log_rotation
    setup_systemd
    write_manifest
    print_summary
}

main "$@"
