# `x/rollup`

This module implements rollup-chain specific logic.

## Deposits

Unlike other cosmos-sdk modules, its state mutation doesn't go through regular tx flow. Instead, states are mutated in
response to L1 deposit txs, and do not need L2 tx signatures.

Sequencer and verifiers must include L1 deposit txs in L2 blocks without any modification.

## Withdrawals

Withdrawals are initiated by L2 users through the rollup module. If a valid withdrawal request is submitted, the user's
L2 ETH is burnt through the bank module. Monomer will then send an L2 state commitment to L1 through the OP Stack and
the user will be able to prove and finalize their withdrawal.

## State

[L1 attributes](https://specs.optimism.io/protocol/ecotone/l1-attributes.html) are stored in this module. Other L2 clients can reference this module to get L1 info for their verifications.

L1 user deposit txs are applied to other modules like `x/bank` and do not mutate this module's state. The rollup module only serves as a gatekeeper for event logging.
