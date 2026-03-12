# Changelog

All notable changes to GoStratumEngine are documented here. Each section corresponds to a released version. 

---

## v1.0.5

### Float Diff Below One — Firmware-Safe Float Difficulty

New `float_diff_below_one` vardiff setting (default `true`). When `float_diff` is enabled, float precision is now restricted to sub-1 difficulty values only — difficulty >= 1 is rounded to integer. This prevents firmware precision issues on Canaan Nano3S and other ASIC devices where float difficulty at high magnitudes (100K+) caused CRC/COM_CRC errors due to float32 limitations in embedded firmware. Set to `false` to restore the previous behavior (float at all magnitudes).

The rounding logic is centralized in a new `roundDifficulty()` method with three modes:
- `float_diff` off: always round to integer (min 1)
- `float_diff` on + `float_diff_below_one` on: float for sub-1, integer for >= 1
- `float_diff` on + `float_diff_below_one` off: float at configured precision for all values

**New vardiff setting:**
- `float_diff_below_one` — only use float difficulty for sub-1 values, integer for >= 1 (default `true`)

---

## v1.0.4

### Mid-Block VarDiff — ASIC Miner Compatibility Fix

When `on_new_block` is set to `false`, VarDiff difficulty changes are now sent immediately with a job resend (`clean_jobs=false`). Previously, only `mining.set_difficulty` was queued — many ASIC miners (e.g., bitaxe) silently ignore difficulty changes without a following `mining.notify`, causing repeated VarDiff adjustments in the wrong direction. The pool now sends `mining.set_difficulty` + `mining.notify` (same block template, `clean_jobs=false`) so the miner applies the new difficulty without wasting hash power. Critical for ZMQ-only setups where no intermediate job broadcasts occur between blocks.

---

## v1.0.3

### Generic Coin Support

Any SHA256d coin can now be added directly in `config.json` without code changes. Provide a `coin_definition` block with address version bytes and Bech32 HRP when using a non-built-in `coin_type`. See the [Generic Coin Support](README.md#generic-coin-support) section in the README for details.

### VarDiff Improvements

- **`on_new_block` setting** — Controls when difficulty changes are delivered to miners. When `true` (default), changes are held until a new block arrives, preventing mid-block low-difficulty share bursts. Set to `false` for faster adaptation on slow chains.
- **Accumulation fix** — VarDiff calculation is now skipped when a pending difficulty change is already queued, preventing runaway difficulty estimates while waiting for delivery.
- **Window reset on delivery** — The share measurement window is cleared when a pending difficulty is applied, ensuring the new difficulty starts with clean data.
- **Full diagnostic logging** — VarDiff decisions are logged at DEBUG level with diagnostics (share count, average time, target range, calculated/clamped diff, change percentage).

### Low-Diff Share Grace Period

New `low_diff_share_grace` stratum setting (default 5 seconds). After any difficulty change (vardiff, `suggest_difficulty`, or password-based), shares meeting the previous difficulty are accepted within the grace window. This prevents rejected shares during difficulty transitions.

### Configuration Changes

**New stratum settings:**
- `low_diff_share_grace` — seconds to accept shares at previous difficulty after a change (default `5`)

**New vardiff settings:**
- `on_new_block` — only deliver difficulty changes on clean jobs (default `true`)

---

## v1.0.2

- Initial public release
- Multi-coin support: BTC, BC2, BCH, DGB, XEC
- Pool and Solo mining modes
- Variable Difficulty (VarDiff)
- Stale share grace period
- mining.suggest_difficulty support
- Password-based difficulty
- Version Rolling (BIP310)
- Server-side Ping
- ZMQ block notifications
- Metrics API
- Developer donation system
