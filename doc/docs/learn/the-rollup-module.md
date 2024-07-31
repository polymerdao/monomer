---
sidebar_position: 1
---

# The `x/rollup` Module
The `x/rollup` module implements rollup-chain specific logic.

Unlike other Cosmos SDK modules, its state mutation doesn't go through regular transaction flow. Instead, states are mutated
in response to L1 deposit transactions, and do not need L2 transaction signatures.

Sequencer and verifiers must include L1 deposit transactions in L2 blocks without any modification.

## State
The `rollup` module stores L1 system info. Other L2 clients can reference this module to get L1 info for their
verifications. L1 user deposit transactions are applied to other modules like `x/bank`, and do not mutate this module's state.
This module only serves as a gatekeeper for event logging.
