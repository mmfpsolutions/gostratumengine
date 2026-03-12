# GoStratumEngine

An open-source Stratum V1 mining pool engine written in Go. Clean-room implementation developed independently. For enterprise features, see [GoSlimStratum](https://www.mmfpsolutions.io).

GoStratumEngine is provided free of charge under the GPLv3 license. By default, the engine contributes 1% of solved blocks to the development team to help fund ongoing development. This applies to both pool and solo mining modes. Please consider leaving this contribution enabled if you are running GoStratumEngine, or contributing directly to the authors listed in `pkg/engine/AUTHORS`. The donation can be disabled or adjusted in `config.json` — see the [Donation](#donation) section below.

## Get Started Fast

Want to get mining as quickly as possible? The **bundle installer** sets up everything you need — GSE, a crypto node, the web dashboard, log rotation, and systemd services — in one command:

```bash
sudo bash -c "$(curl -sSL https://get.gostratumengine.io/scripts/install-cli.sh)"
```

It walks you through coin selection (BTC, BCH, or DGB), configures RPC credentials, sets up blockchain pruning, and gets you running. See the [Bundle Installer README](shell-scripts/bundle-installer/README.md) for details.

If you prefer to set things up manually, keep reading.

## Features

- **Multi-coin support** — BTC, BC2, BCH, DGB, XEC, plus any SHA256d coin via generic coin definitions
- **Pool and Solo mining modes** — pool mode uses a single payout address; solo mode lets each miner provide their own wallet address
- **Variable Difficulty (VarDiff)** — automatically adjusts per-miner difficulty based on hashrate, sent with job updates (not mid-share)
- **Stale share grace period** — configurable window (default 5s) to accept in-flight shares after a new block
- **Low-diff share grace period** — configurable window (default 5s) to accept shares at the previous difficulty after a difficulty change
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
        "stale_share_grace": 5,
        "low_diff_share_grace": 5
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
        "variance_percent": 30,
        "on_new_block": true
      }
    }
  }
}
```

### Generic Coin Support

Any SHA256d coin can be added directly in `config.json` without code changes. When `coin_type` is not a built-in (`bitcoin`, `bitcoinii`, `bitcoincash`, `digibyte`, `ecash`), provide a `coin_definition` block with the coin's address parameters:

```json
{
  "coins": {
    "FB": {
      "enabled": true,
      "coin_type": "fractal",
      "coin_definition": {
        "name": "Fractal Bitcoin",
        "symbol": "FB",
        "segwit": true,
        "address": {
          "bech32": {
            "hrp": { "mainnet": "bc", "testnet": "tb" }
          },
          "base58": {
            "p2pkh": { "mainnet": 0, "testnet": 111 },
            "p2sh": { "mainnet": 5, "testnet": 196 }
          }
        }
      },
      "node": { "host": "127.0.0.1", "port": 8332, "..." : "..." },
      "stratum": { "..." : "..." },
      "mining": { "..." : "..." },
      "vardiff": { "..." : "..." }
    }
  }
}
```

**`coin_definition` fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Display name (e.g., "Fractal Bitcoin") |
| `symbol` | Yes | Ticker symbol, 2-10 uppercase characters (e.g., "FB") |
| `segwit` | No | Whether the coin supports SegWit (default `false`) |
| `address.base58.p2pkh` | Yes | P2PKH version bytes: `{ "mainnet": N, "testnet": N }` (0-255) |
| `address.base58.p2sh` | No | P2SH version bytes (same format as p2pkh) |
| `address.bech32.hrp` | If segwit | Bech32 human-readable prefix: `{ "mainnet": "bc", "testnet": "tb" }` |

**What generic coins cover:** SHA256d coins with standard Base58 and/or Bech32 addresses — this includes most Bitcoin forks and clones.

**What requires built-in support:** CashAddr address formats (BCH/XEC), custom coinbase splits (eCash miner fund/staking rewards), non-SHA256d algorithms, and RTT validation (eCash).

### Mining Modes

**Pool mode** (`"mode": "pool"`) — All block rewards are sent to a single payout address configured in `mining.address`. GoStratumEngine does not include a payout system — reward distribution to individual miners must be handled externally.

**Solo mode** (`"mode": "solo"`) — Each miner provides their wallet address as their worker name (e.g., `bc1qxyz.worker1`). Block rewards go directly to the miner's address. No `mining.address` needed.

### Variable Difficulty (VarDiff)

VarDiff automatically adjusts each miner's difficulty based on their hashrate. It uses a rolling window of share timestamps to calculate the average time between shares, then adjusts difficulty to match the configured target time.

```json
"vardiff": {
  "enabled": true,
  "min_diff": 512,
  "max_diff": 32768,
  "target_time": 15,
  "retarget_time": 300,
  "variance_percent": 30,
  "float_diff": false,
  "float_diff_below_one": true,
  "float_precision": 2,
  "on_new_block": true
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable VarDiff for this coin |
| `min_diff` | `512` | Minimum difficulty floor |
| `max_diff` | `32768` | Maximum difficulty ceiling |
| `target_time` | `15` | Target seconds between shares |
| `retarget_time` | `300` | Minimum seconds between difficulty adjustments |
| `variance_percent` | `30` | Acceptable deviation from target before adjusting (%) |
| `float_diff` | `false` | Allow fractional difficulty values |
| `float_diff_below_one` | `true` | Only use float difficulty for sub-1 values, integer for >= 1. Prevents firmware precision issues on Canaan/AxeOS devices at high difficulty magnitudes. Only applies when `float_diff` is `true`. |
| `float_precision` | `2` | Decimal places when `float_diff` is true |
| `on_new_block` | `true` | Only deliver difficulty changes with new block notifications (clean jobs). When `false`, changes are delivered on any job broadcast including routine template refreshes. |

When `on_new_block` is `true` (the default), difficulty changes are calculated on share acceptance but held until the next new block arrives. This prevents mid-block difficulty drops that could produce bursts of low-difficulty shares. Set to `false` on slow chains where you want faster difficulty adaptation — changes will then be delivered on the next template refresh (controlled by `template_refresh_interval`).

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
- A full node for each coin you want to mine (Bitcoin Core, Bitcoin II Core, DigiByte Core, Bitcoin Cash Node, Bitcoin ABC, or any SHA256d coin node)
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
