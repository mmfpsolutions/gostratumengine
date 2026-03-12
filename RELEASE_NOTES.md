## What's New in v1.0.5

### Float Diff Below One — Firmware-Safe Float Difficulty

When `float_diff` is enabled, the pool can now restrict float precision to sub-1 difficulty values only, sending integer difficulty for values >= 1. This is controlled by the new `float_diff_below_one` setting (default: `true`).

This resolves a firmware compatibility issue discovered on Canaan Nano3S (and potentially other ASIC devices) where float difficulty at high magnitudes (100K+) caused CRC/COM_CRC errors on the chip data bus. Embedded miner firmware typically uses float32 internally, which only supports ~7 significant digits. A difficulty like `103297.0000` exceeds float32 precision, causing the firmware to misinterpret the value and produce hardware errors.

With `float_diff_below_one` enabled:
- Difficulty < 1 (e.g., `0.0038`, `0.022`) — sent as float with configured precision. Ideal for tiny miners like NerdMiner and ESP32 devices.
- Difficulty >= 1 (e.g., `4096`, `103297`) — rounded to integer. Safe for all ASIC firmware including Canaan and AxeOS devices that truncate or can't process float difficulty.

To use the previous behavior (float at all magnitudes), set `"float_diff_below_one": false` in your vardiff config.
