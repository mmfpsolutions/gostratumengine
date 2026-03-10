## What's New in v1.0.4

### Mid-Block VarDiff — ASIC Miner Compatibility Fix

When `on_new_block` is set to `false`, VarDiff difficulty changes are now sent immediately with a job resend (`clean_jobs=false`). Previously, the pool sent only `mining.set_difficulty` and stored the pending change for the next job broadcast. Many ASIC miners (e.g., bitaxe) silently ignore `mining.set_difficulty` unless it is followed by a `mining.notify` — causing the miner to keep submitting shares at the old difficulty, which triggered repeated VarDiff adjustments in the wrong direction.

The fix sends `mining.set_difficulty` followed by `mining.notify` with the current block template and `clean_jobs=false`. The miner acknowledges the new difficulty and continues hashing the same block — no hash power is wasted. The existing `prevDiff` grace period handles any in-flight shares at the old difficulty.

This is especially important when polling is disabled and only ZMQ is used for block notifications, since there are no intermediate job broadcasts between blocks to flush pending difficulty changes.
