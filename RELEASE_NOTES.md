## What's New in v1.0.6

### Taproot Addresses

You can now mine to a Taproot address (`bc1p...`, `dgb1p...`) on Bitcoin, BitcoinII, DigiByte and generic SegWit coins, as the pool payout address or as a miner's address in solo mode. Earlier versions rejected them.

Address checking is also stricter and more consistent. An address is accepted only if GoStratumEngine can actually pay it, so an unsupported address is refused when the miner connects or the pool starts, not later when a job is built. Addresses that worked in v1.0.5 work the same way.

If you run a generic coin, only use a Taproot address when Taproot is active on that chain.

### DigiDollar Support for DigiByte

DigiByte pools now ask the node for DigiDollar-aware block templates and carry the node's oracle commitment in the coinbase. Your blocks can include DigiDollar mint and redeem transactions, and collect their fees.

There is nothing to configure. It's always on for DigiByte, and it works with any node: if the node doesn't provide an oracle commitment, GoStratumEngine builds exactly the same block as before.

### DigiByte Nodes No Longer Need `algo=sha256d`

GoStratumEngine now tells the DigiByte node which algorithm it is mining. Before, it relied on the node's own `algo=` setting, and a node set to anything other than `sha256d` (including a node with no setting, which defaults to scrypt) made every share look like a block and every block submission fail with `high-hash`. The setting no longer matters.

### Donation Addresses

The BTC, BCH and DGB donation addresses have been updated, and Bitcoin Silver (BTCS) has been added for pools that run it as a generic coin with the config key `BTCS`.

### Upgrading

No configuration changes are needed.
