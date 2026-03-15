# GoStratumEngine Bundle Installer

Interactive CLI installer and uninstaller for deploying GoStratumEngine and its dependencies on a Linux server.

## What It Installs:

| Component | Description | Path |
|-----------|-------------|------|
| **GoStratumEngine** | Stratum V1 mining pool engine | `{base}/gse/` |
| **Crypto Node** | BTC (Bitcoin Knots), BCH (Bitcoin Cash Node), or DGB (DigiByte) | `{base}/{coin}/` |
| **Web Dashboard** | Browser-based pool monitoring UI | `{base}/gse-webui/` |
| **Log Rotation** | Automatic log file management via cron | `{base}/log_rotation/` |
| **Systemd Services** | Auto-start on boot (Linux only) | `/etc/systemd/system/` |

The default base path is `/mining`.

## Requirements

- Linux (Ubuntu/Debian or RHEL/CentOS)
- Root access (sudo)
- `curl`, `openssl`, `tar`, `xxd`
- `python3` (only if installing the web dashboard)

## Install

```bash
sudo bash -c "$(curl -sSL https://get.gostratumengine.io/shell-scripts/bundle-installer/install-cli.sh)"
```

The installer will interactively prompt for:

1. **Components** — GSE only, GSE + Node, or GSE + Node + Dashboard
2. **Coin** — BTC, BCH, or DGB (if installing a node)
3. **Base path** — where to install (default: `/mining`)
4. **RPC credentials** — username and password for node communication (auto-generates password if left blank)
5. **Blockchain pruning** — 550MB (recommended), 10GB, 50GB, or full node
6. **Log rotation** — automatic log management via cron
7. **Systemd services** — auto-start on boot

A confirmation summary is displayed before any changes are made.


## Uninstall

```bash
sudo bash -c "$(curl -sSL https://get.gostratumengine.io/shell-scripts/bundle-installer/uninstall-cli.sh)"
```

The uninstaller reads the installation manifest (`.gse-manifest.json`) to know exactly what was installed. It offers two removal modes:

1. **Remove everything** — all software and blockchain data
2. **Remove software only** — keeps blockchain data intact (avoids days/weeks of re-syncing)

Both modes will:
- Stop all running services (node, GSE, dashboard)
- Remove systemd service files
- Remove crontab entries
- Clean up installed files

## Files

| File | Description |
|------|-------------|
| `install-cli.sh` | Interactive installer (~1000 lines) |
| `uninstall-cli.sh` | Manifest-driven uninstaller (~280 lines) |

## Manifest

The installer writes a JSON manifest to `{base}/.gse-manifest.json` that tracks everything installed — components, versions, paths, binary names, and configuration. The uninstaller depends on this file. Do not delete it.

## Directory Layout

After a full install with DGB:

```
/mining/
  .gse-manifest.json        # Installation manifest
  gse/
    gostratumengine          # GSE binary
    config.json              # GSE configuration
    gse.sh                   # Start/stop/restart/status
  dgb/
    digibyted                # Node daemon
    digibyte-cli             # Node CLI
    service.sh               # Start/stop/restart/status
    data/
      digibyte.conf          # Node configuration
      (blockchain data)
  gse-webui/
    server.py                # Dashboard server
    dashboard.sh             # Start/stop/status
  log_rotation/
    log_rotation.sh          # Rotation script
    log_rotation.conf        # Rotation configuration
```

## License

Copyright 2026 Scott Walter, MMFP Solutions LLC. Licensed under the GNU General Public License v3.
