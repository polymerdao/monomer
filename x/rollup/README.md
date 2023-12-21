# `x/rollup`

This module implements rollup-chain specific logic.

Unlike other cosmos-sdk module, its state mutation doesn't go through regular tx flow. Instead, states are mutated in
response to L1 deposit txs, and do not need L2 tx signatures.

Sequencer and verifiers must include L1 deposit txs in L2 blocks without any modification.

## State

L1 system info are stored in this module. Other L2 clients can reference this module to get L1 info for their verifications.

L1 user deposit txs are applied to other modules like `x/bank` and do not mutate this module's state. The rollup module only serves as a gatekeeper for event logging.
