# GoStratumEngine - Project Conventions

## Overview
Open-source Stratum V1 mining pool engine written in Go. Supports BTC, BCH, DGB, and XEC.
GPL v3 licensed. Module: `github.com/mmfpsolutions/gostratumengine`

## Architecture
- `cmd/gostratumengine/` - Entry point binary
- `pkg/` - All packages (importable by external projects)
  - `config/` - JSON config loading and validation
  - `logging/` - Module-tagged leveled logger (stdout)
  - `coin/` - Coin interface, registry, 4 coin implementations, address encoding (Bech32, Base58, CashAddr)
  - `coinbase/` - Coinbase TX construction, merkle trees, script builders
  - `noderpc/` - JSON-RPC client for blockchain nodes + ZMQ subscriber
  - `stratum/` - TCP stratum server, session management, VarDiff, protocol types
  - `engine/` - Job manager, share validator, coin runner, top-level orchestrator
  - `metrics/` - In-memory stats + HTTP API

## Key Design Decisions
- All `pkg/` not `internal/` — packages are importable by other projects
- No database — all state is in-memory
- No CGO for ZMQ — uses pure-Go `go-zeromq/zmq4`
- Single Coin interface (no sub-interfaces) with registry pattern
- Clean-room implementation — does NOT copy from GoSlimStratum

## Build & Test
```
make build        # Build for current platform
make build-all    # Build all platforms (linux/darwin, amd64/arm64)
make test         # Run tests
go vet ./...      # Lint
```

## Configuration
Copy `config.example.json` to `config.json` and edit. See design-documents/ for full spec.

## Code Style
- Standard Go formatting (gofmt)
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Module-tagged logging via `logging.New(module)`
- No global state in coin implementations — config passed through engine
