#!/usr/bin/env bash
#
# Copyright 2026 Scott Walter, MMFP Solutions LLC
#
# This program is free software; you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation; either version 3 of the License, or (at your option)
# any later version.  See LICENSE for more details.
# log_rotation.sh - Rotate logs safely without stopping services.
# Uses cp + truncate (cat /dev/null) so running processes keep writing
# to the same file descriptor.
#
# Usage:
#   ./log_rotation.sh                            # uses default config
#   ./log_rotation.sh /etc/log_rotation.conf     # custom config
#   ./log_rotation.sh --dry-run                  # preview with default config
#   ./log_rotation.sh --dry-run /etc/myapp.conf  # preview with custom config
#
# Crontab example (run every hour):
#   0 * * * * /opt/scripts/log_rotation.sh /etc/log_rotation.conf >> /var/log/log_rotation.log 2>&1
#
# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------
# Config file format (one log entry per line, blank lines and # comments ok):
#   LOG_PATH | MAX_SIZE_MB | MAX_AGE_DAYS | COMPRESS | ARCHIVE_DIR
#
# Example config file:
#   /var/log/myapp/app.log      | 50  | 30 | yes | /var/log/myapp/archive
#   /var/log/myapp/error.log    | 25  | 14 | yes | /var/log/myapp/archive
#   /var/log/nginx/access.log   | 100 | 7  | no  | /var/log/nginx/rotated
#
# Fields:
#   LOG_PATH     - absolute path to the log file
#   MAX_SIZE_MB  - rotate when file exceeds this size (in MB); 0 = size check disabled
#   MAX_AGE_DAYS - delete rotated logs older than this many days; 0 = keep forever
#   COMPRESS     - yes/no — gzip the rotated copy
#   ARCHIVE_DIR  - directory to store rotated logs (created automatically)
# -----------------------------------------------------------------------------

set -euo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly TIMESTAMP="$(date '+%Y%m%d_%H%M%S')"
readonly DEFAULT_CONFIG="/etc/log_rotation.conf"
readonly LOCK_DIR="/tmp/log_rotation.lock"
DRY_RUN=false

# -----------------------------------------------------------------------------
# Logging helpers
# -----------------------------------------------------------------------------
log_info()  { echo "[$(date '+%F %T')] [INFO]  $*"; }
log_warn()  { echo "[$(date '+%F %T')] [WARN]  $*" >&2; }
log_error() { echo "[$(date '+%F %T')] [ERROR] $*" >&2; }
log_dry()   { echo "[$(date '+%F %T')] [DRY]   $*"; }

# -----------------------------------------------------------------------------
# Locking — prevent overlapping runs
# -----------------------------------------------------------------------------
acquire_lock() {
    if mkdir "$LOCK_DIR" 2>/dev/null; then
        trap 'rm -rf "$LOCK_DIR"' EXIT
    else
        log_error "Another instance is running (lock: $LOCK_DIR). Exiting."
        exit 1
    fi
}

# -----------------------------------------------------------------------------
# Validate a config line and return cleaned fields
# -----------------------------------------------------------------------------
parse_config_line() {
    local line="$1"

    # Strip inline comments and trim whitespace
    line="${line%%#*}"
    [[ -z "${line// /}" ]] && return 1

    # Split on pipe delimiter
    IFS='|' read -r log_path max_size max_age compress archive_dir <<< "$line"

    # Trim whitespace from each field
    log_path="$(echo "$log_path" | xargs)"
    max_size="$(echo "$max_size" | xargs)"
    max_age="$(echo "$max_age" | xargs)"
    compress="$(echo "$compress" | xargs | tr '[:upper:]' '[:lower:]')"
    archive_dir="$(echo "$archive_dir" | xargs)"

    # Validate required fields
    if [[ -z "$log_path" || -z "$max_size" || -z "$max_age" || -z "$compress" || -z "$archive_dir" ]]; then
        log_warn "Incomplete config line: $1"
        return 1
    fi

    if ! [[ "$max_size" =~ ^[0-9]+$ ]]; then
        log_warn "Invalid MAX_SIZE_MB '$max_size' for $log_path"
        return 1
    fi

    if ! [[ "$max_age" =~ ^[0-9]+$ ]]; then
        log_warn "Invalid MAX_AGE_DAYS '$max_age' for $log_path"
        return 1
    fi

    if [[ "$compress" != "yes" && "$compress" != "no" ]]; then
        log_warn "COMPRESS must be yes/no, got '$compress' for $log_path"
        return 1
    fi

    # Export parsed values via global variables (caller reads these)
    PARSED_LOG_PATH="$log_path"
    PARSED_MAX_SIZE="$max_size"
    PARSED_MAX_AGE="$max_age"
    PARSED_COMPRESS="$compress"
    PARSED_ARCHIVE_DIR="$archive_dir"
    return 0
}

# -----------------------------------------------------------------------------
# Check if a log file needs rotation based on size
# -----------------------------------------------------------------------------
needs_rotation() {
    local log_path="$1"
    local max_size_mb="$2"

    # Size check disabled
    if [[ "$max_size_mb" -eq 0 ]]; then
        return 1
    fi

    if [[ ! -f "$log_path" ]]; then
        return 1
    fi

    local file_size_bytes
    file_size_bytes="$(stat -c%s "$log_path" 2>/dev/null || stat -f%z "$log_path" 2>/dev/null)"
    local max_size_bytes=$(( max_size_mb * 1024 * 1024 ))

    if [[ "$file_size_bytes" -ge "$max_size_bytes" ]]; then
        return 0
    fi

    return 1
}

# -----------------------------------------------------------------------------
# Rotate a single log file: cp then truncate
# -----------------------------------------------------------------------------
rotate_log() {
    local log_path="$1"
    local archive_dir="$2"
    local compress="$3"

    local base_name
    base_name="$(basename "$log_path")"
    local rotated_name="${base_name}.${TIMESTAMP}"
    local rotated_path="${archive_dir}/${rotated_name}"

    local file_size
    file_size="$(du -h "$log_path" | cut -f1)"

    if [[ "$DRY_RUN" == true ]]; then
        log_dry "Would rotate: $log_path ($file_size) -> $rotated_path"
        [[ "$compress" == "yes" ]] && log_dry "Would compress: ${rotated_path}.gz"
        return
    fi

    # Ensure archive directory exists
    mkdir -p "$archive_dir"

    # Copy the log to the archive
    cp -p "$log_path" "$rotated_path"

    # Truncate the original — process keeps its file descriptor open
    cat /dev/null > "$log_path"

    log_info "Rotated: $log_path -> $rotated_path ($file_size)"

    # Compress if requested
    if [[ "$compress" == "yes" ]]; then
        gzip "$rotated_path"
        log_info "Compressed: ${rotated_path}.gz"
    fi
}

# -----------------------------------------------------------------------------
# Purge rotated logs older than retention period
# -----------------------------------------------------------------------------
purge_old_logs() {
    local archive_dir="$1"
    local max_age_days="$2"
    local base_name="$3"

    # Retention disabled
    if [[ "$max_age_days" -eq 0 ]]; then
        return
    fi

    if [[ ! -d "$archive_dir" ]]; then
        return
    fi

    local count
    count="$(find "$archive_dir" -maxdepth 1 -name "${base_name}.*" -type f -mtime +"$max_age_days" | wc -l)"

    if [[ "$count" -gt 0 ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            log_dry "Would purge $count rotated file(s) older than ${max_age_days}d from $archive_dir:"
            find "$archive_dir" -maxdepth 1 -name "${base_name}.*" -type f -mtime +"$max_age_days" -exec basename {} \; | while read -r f; do
                log_dry "  $f"
            done
        else
            find "$archive_dir" -maxdepth 1 -name "${base_name}.*" -type f -mtime +"$max_age_days" -delete
            log_info "Purged $count rotated file(s) older than ${max_age_days}d from $archive_dir"
        fi
    fi
}

# -----------------------------------------------------------------------------
# Process all entries in the config file
# -----------------------------------------------------------------------------
process_config() {
    local config_file="$1"
    local rotated=0
    local errors=0

    while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip blank lines and full-line comments
        [[ -z "${line// /}" || "$line" =~ ^[[:space:]]*# ]] && continue

        if ! parse_config_line "$line"; then
            (( errors++ ))
            continue
        fi

        local log_path="$PARSED_LOG_PATH"
        local max_size="$PARSED_MAX_SIZE"
        local max_age="$PARSED_MAX_AGE"
        local compress="$PARSED_COMPRESS"
        local archive_dir="$PARSED_ARCHIVE_DIR"

        if [[ ! -f "$log_path" ]]; then
            log_warn "Log file not found: $log_path — skipping"
            continue
        fi

        # Always run retention cleanup
        purge_old_logs "$archive_dir" "$max_age" "$(basename "$log_path")"

        # Check size threshold
        if needs_rotation "$log_path" "$max_size"; then
            rotate_log "$log_path" "$archive_dir" "$compress"
            (( rotated++ ))
        fi

    done < "$config_file"

    log_info "Rotation complete: $rotated file(s) rotated, $errors config error(s)"
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
main() {
    local config_file="$DEFAULT_CONFIG"

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run|-n)
                DRY_RUN=true
                shift
                ;;
            *)
                config_file="$1"
                shift
                ;;
        esac
    done

    if [[ ! -f "$config_file" ]]; then
        log_error "Config file not found: $config_file"
        echo ""
        echo "Create a config file with one entry per line:"
        echo "  LOG_PATH | MAX_SIZE_MB | MAX_AGE_DAYS | COMPRESS | ARCHIVE_DIR"
        echo ""
        echo "Example:"
        echo "  /var/log/myapp/app.log | 50 | 30 | yes | /var/log/myapp/archive"
        exit 1
    fi

    acquire_lock

    if [[ "$DRY_RUN" == true ]]; then
        log_info "DRY RUN — no changes will be made (config: $config_file)"
    else
        log_info "Starting log rotation (config: $config_file)"
    fi

    process_config "$config_file"
}

main "$@"
