# GoStratumEngine

An open-source Stratum V1 mining pool engine written in Go. Clean-room implementation inspired by [GoSlimStratum](https://github.com/mmfpsolutions/goslimstratum).

## Features

- **Multi-coin support** — BTC, BCH, DGB, and XEC
- **Pool and Solo mining modes** — pool mode uses a single payout address; solo mode lets each miner provide their own wallet address
- **Variable Difficulty (VarDiff)** — automatically adjusts per-miner difficulty based on hashrate
- **Version Rolling** — BIP310 support for ASICBoost-capable miners
- **Server-side Ping** — configurable mining.ping/pong keepalive cycle
- **ZMQ block notifications** — instant new block detection via pure-Go ZMQ (no CGO)
- **Address format support** — Bech32 (P2WPKH/P2WSH), Base58 (P2PKH/P2SH), and CashAddr
- **Metrics API** — HTTP endpoints for pool stats, per-worker metrics, and live session info
- **No database** — all state is in-memory for simplicity and performance

## Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |

## Quick Start

### From Release

Download the latest binary from [Releases](https://github.com/mmfpsolutions/gostratumengine/releases), then:

```bash
cp config.example.json config.json
# Edit config.json with your node and mining settings
./gostratumengine
```

### From Source

```bash
git clone https://github.com/mmfpsolutions/gostratumengine.git
cd gostratumengine
make build
cp config.example.json config.json
# Edit config.json
./bin/gostratumengine
```

## Configuration

Copy `config.example.json` to `config.json` and edit for your setup. Key sections:

```json
{
  "pool_name": "MyPool",
  "log_level": "info",
  "api_port": 8080,
  "coins": {
    "BTC": {
      "enabled": true,
      "coin_type": "bitcoin",
      "node": {
        "host": "127.0.0.1",
        "port": 8332,
        "username": "rpcuser",
        "password": "rpcpassword",
        "zmq_enabled": true,
        "zmq_hashblock": "tcp://127.0.0.1:28332"
      },
      "stratum": {
        "host": "0.0.0.0",
        "port": 3333,
        "difficulty": 1024,
        "ping_enabled": true,
        "ping_interval": 30
      },
      "mining": {
        "mode": "pool",
        "address": "bc1qYOURADDRESSHERE",
        "network": "mainnet",
        "coinbase_text": "MyPool/GoStratumEngine",
        "extranonce_size": 8
      },
      "vardiff": {
        "enabled": true,
        "min_diff": 512,
        "max_diff": 32768,
        "target_time": 15,
        "retarget_time": 300,
        "variance_percent": 30
      }
    }
  }
}
```

### Mining Modes

**Pool mode** (`"mode": "pool"`) — All miners share a single payout address configured in `mining.address`. Traditional pool operation.

**Solo mode** (`"mode": "solo"`) — Each miner provides their wallet address as their worker name (e.g., `bc1qxyz.worker1`). Block rewards go directly to the miner's address. No `mining.address` needed.

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /stats` | Pool-wide statistics (shares, blocks, uptime) |
| `GET /miners` | Per-worker stats and live session info |

## Build Targets

```bash
make build            # Build for current platform
make build-all        # Build all platforms (linux/darwin, amd64/arm64)
make test             # Run tests
make lint             # Run go vet
make clean            # Remove build artifacts
```

## Project Structure

```
cmd/gostratumengine/  Entry point binary
pkg/
  config/             JSON config loading and validation
  logging/            Module-tagged leveled logger
  coin/               Coin interface, registry, address encoding
  coinbase/           Coinbase TX construction, merkle trees
  noderpc/            JSON-RPC client for blockchain nodes + ZMQ
  stratum/            TCP stratum server, session management, VarDiff
  engine/             Job manager, share validator, orchestrator
  metrics/            In-memory stats + HTTP API
```

## Requirements

- Go 1.25+ (for building from source)
- A full node for each coin you want to mine (Bitcoin Core, DigiByte Core, Bitcoin Cash Node, Bitcoin ABC)
- ZMQ enabled on your node (recommended for instant block detection)

## License

GPL v3 — see [LICENSE](LICENSE) for details.

Copyright 2026 Scott Walter, MMFP Solutions LLC
