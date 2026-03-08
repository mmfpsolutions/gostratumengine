# GoStratumEngine

An open-source Stratum V1 mining pool engine written in Go. Clean-room implementation developed independently. For enterprise features, see [GoSlimStratum](https://www.mmfpsolutions.io).

GoStratumEngine is provided free of charge under the GPLv3 license. By default, the engine contributes 1% of solved blocks to the development team to help fund ongoing development. This applies to both pool and solo mining modes. Please consider leaving this contribution enabled if you are running GoStratumEngine, or contributing directly to the authors listed in `pkg/engine/AUTHORS`. The donation can be disabled or adjusted in `config.json` — see the [Donation](#donation) section below.

## Features

- **Multi-coin support** — BTC, BC2, BCH, DGB, and XEC
- **Pool and Solo mining modes** — pool mode uses a single payout address; solo mode lets each miner provide their own wallet address
- **Variable Difficulty (VarDiff)** — automatically adjusts per-miner difficulty based on hashrate, sent with job updates (not mid-share)
- **Stale share grace period** — configurable window (default 5s) to accept in-flight shares after a new block
- **mining.suggest_difficulty** — optionally honor miner-requested difficulty (configurable per coin)
- **Password-based difficulty** — miners can set difficulty via `d=XXX` in the authorize password field
- **Version Rolling** — BIP310 support for ASICBoost-capable miners
- **Server-side Ping** — configurable mining.ping/pong keepalive cycle
- **ZMQ block notifications** — instant new block detection via pure-Go ZMQ (no CGO)
- **Address format support** — Bech32 (P2WPKH/P2WSH), Base58 (P2PKH/P2SH), and CashAddr
- **Metrics API** — HTTP endpoints for pool stats, per-worker metrics, and live session info
- **No database** — all state is in-memory for simplicity and performance

## DigiByte (DGB) Support

Most open-source stratum implementations only support Bitcoin. GoStratumEngine includes native DigiByte support:

- **SegWit-aware coinbase construction** — proper witness commitment handling for DGB's SegWit transactions
- **Native address validation** — Bech32 (`dgb1...`), P2PKH, and P2SH with correct DGB version bytes (mainnet and testnet)
- **Correct address script generation** — P2WPKH, P2WSH, P2PKH, and P2SH output scripts for DGB's address formats

DigiByte uses SHA-256d for its SHA-256 algorithm slot, making it compatible with standard Bitcoin ASIC miners. GoStratumEngine handles DGB's unique address encoding and SegWit implementation out of the box.

## eCash (XEC) Support

eCash has unique consensus requirements that most stratum implementations ignore entirely. GoStratumEngine is Avalanche-aware:

- **Real Time Target (RTT) validation** — computes and validates the eCash RTT before submitting blocks, preventing wasted submissions that the network would reject
- **Avalanche chain reorg detection** — verifies the chain tip hasn't changed between template fetch and block submission, avoiding submissions on parked chains
- **Block submission cooldown** — 30-second cooldown after submitting a block to prevent submitting on Avalanche-parked chain forks
- **Miner Fund & Staking Rewards** — automatically includes mandatory miner fund and staking reward outputs in the coinbase transaction as required by the eCash protocol
- **CashAddr address support** — native `ecash:` prefix CashAddr validation and script generation, plus legacy Base58 fallback

These protections are critical for eCash mining. Without RTT validation and Avalanche awareness, a stratum server will submit blocks that get rejected by the network — wasting miner effort and missing block rewards.

## Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |

## Quick Start

### From Release

Download the latest binary for your platform:

```bash
# Linux (amd64)
curl -LO https://github.com/mmfpsolutions/gostratumengine/releases/latest/download/gostratumengine-linux-amd64
chmod +x gostratumengine-linux-amd64

# Linux (arm64)
curl -LO https://github.com/mmfpsolutions/gostratumengine/releases/latest/download/gostratumengine-linux-arm64
chmod +x gostratumengine-linux-arm64

# macOS (Apple Silicon)
curl -LO https://github.com/mmfpsolutions/gostratumengine/releases/latest/download/gostratumengine-darwin-arm64
chmod +x gostratumengine-darwin-arm64

# macOS (Intel)
curl -LO https://github.com/mmfpsolutions/gostratumengine/releases/latest/download/gostratumengine-darwin-amd64
chmod +x gostratumengine-darwin-amd64
```

Then configure and run:

```bash
curl -LO https://github.com/mmfpsolutions/gostratumengine/releases/latest/download/config.example.json
cp config.example.json config.json
# Edit config.json with your node and mining settings
./gostratumengine-linux-amd64
```

All releases are available at [Releases](https://github.com/mmfpsolutions/gostratumengine/releases).

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
        "ping_interval": 30,
        "accept_suggest_diff": false,
        "stale_share_grace": 5
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

**Pool mode** (`"mode": "pool"`) — All block rewards are sent to a single payout address configured in `mining.address`. GoStratumEngine does not include a payout system — reward distribution to individual miners must be handled externally.

**Solo mode** (`"mode": "solo"`) — Each miner provides their wallet address as their worker name (e.g., `bc1qxyz.worker1`). Block rewards go directly to the miner's address. No `mining.address` needed.

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/stats` | Pool-wide statistics (shares, blocks, uptime) |
| `GET /api/v1/miners` | Per-worker stats and live session info |
| `GET /api/v1/health` | Health check |

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
- A full node for each coin you want to mine (Bitcoin Core, Bitcoin II Core, DigiByte Core, Bitcoin Cash Node, Bitcoin ABC)
- ZMQ enabled on your node (recommended for instant block detection)

## Donation

GoStratumEngine includes an optional developer donation that contributes a small percentage of each block reward to the project authors. This helps fund continued development and maintenance of the project.

- **Enabled by default** at 1% of the block reward
- Applies to both **pool** and **solo** mining modes
- Donation addresses are embedded in the binary (`pkg/engine/AUTHORS`)
- Per-coin, per-network addresses (mainnet and testnet)
- If no donation address exists for a coin/network combination, donation is silently skipped

To adjust or disable, add a `donation` section to your `config.json`:

```json
{
  "donation": {
    "enabled": true,
    "percent": 1.0
  }
}
```

Set `"enabled": false` to disable donations entirely, or adjust `"percent"` to change the amount.

## License

GPL v3 — see [LICENSE](LICENSE) for details.

Copyright 2026 Scott Walter, MMFP Solutions LLC
