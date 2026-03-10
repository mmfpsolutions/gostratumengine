# Changelog

All notable changes to GoStratumEngine are documented here. Each section corresponds to a released version.

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
