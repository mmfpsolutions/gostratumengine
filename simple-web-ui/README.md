# GoStratumEngine Simple Web UI

A lightweight web dashboard for monitoring a running [GoStratumEngine](https://github.com/mmfpsolutions/gostratumengine) instance. No frameworks, no dependencies beyond Python 3.

## Features

- Pool health status indicator
- Pool name, uptime, and aggregate share/block counts
- Per-coin statistics (accepted, rejected, stale shares, blocks found)
- Connected miner details (worker name, difficulty, shares, connection time)
- Auto-refreshes every 10 seconds

## Requirements

- Python 3.6+
- A running GoStratumEngine instance with the API enabled

## Quick Start

```bash
# Start the dashboard
./dashboard.sh start

# Check if it's running
./dashboard.sh status

# Stop it
./dashboard.sh stop
```

Then open [http://localhost:8000](http://localhost:8000) in your browser.

You can also run it directly in the foreground:

```bash
python3 server.py
```

## Configuration

Configuration is done via environment variables:

| Variable | Default | Description |
|---|---|---|
| `API_BASE` | `http://127.0.0.1:8080` | GoStratumEngine API address |
| `LISTEN_PORT` | `8000` | Port the dashboard listens on |

Example with custom settings:

```bash
API_BASE=http://192.168.1.10:8080 LISTEN_PORT=9000 ./dashboard.sh start
```

## How It Works

`server.py` is a single-file Python HTTP server that:

1. Serves the dashboard HTML/CSS/JS at `/`
2. Proxies browser requests from `/api/*` to the GoStratumEngine API at `/api/v1/*`

This proxy approach means the Go engine does not need to be exposed directly to the browser and avoids CORS issues.

## API Endpoints Used

| Dashboard Path | Proxied To | Purpose |
|---|---|---|
| `/api/health` | `/api/v1/health` | Health check |
| `/api/stats` | `/api/v1/stats` | Pool and per-coin statistics |
| `/api/miners` | `/api/v1/miners` | Connected miner details |

## Files

| File | Description |
|---|---|
| `server.py` | Web server and embedded dashboard UI |
| `dashboard.sh` | Start/stop/status management script |
| `dashboard.log` | Log output (created at runtime) |
| `dashboard.pid` | PID file (created at runtime) |

## License

GPLv3 — see [LICENSE](../LICENSE) for details.
