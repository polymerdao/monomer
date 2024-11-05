---
sidebar_position: 3
---

import ThemedImage from '@theme/ThemedImage';

# Withdrawals

Monomer supports ETH withdrawals through the Optimism Portal L1 contract. This is the canonical way to move assets from an OP Stack rollup chain to Ethereum.

:::note
For UX purposes, exiting an ORU via the canonical bridge is impractical and slow. But the exit mechanism must be available for security reasons.
:::

The withdrawal transaction flow is illustrated below. The lifecycle moves in the opposite direction from the Deposits flow - starting at the Cosmos-SDK appchain, processed by the Monomer `x/rollup` module and then Monomer's builder, and finally into the OP-Stack components on L1.

Again, this document focuses on Monomer specific modifications. Full documentation for the withdrawal flow is available in the [OP Stack specs](https://specs.optimism.io/protocol/withdrawals.html).

<ThemedImage
alt="Withdrawal workflow"
sources={{ light: '/img/withdrawals-light.png', dark: '/img/withdrawals-dark.png' }}
/>

## Initiating Withdrawals

The withdrawal process begins with an L2 user submitting a withdrawal transaction to the Cosmos-SDK appchain, with the same data used to initiate a withdrawal on a standard OP Stack. These messages are interpreted by both the `x/rollup` module of the appchain, and the Monomer block builder, which writes necessary data to the [EVM sidecar](/learn/blocks#state).

### L2 Appchain

On the L2 appchain, via custom message types and handlers from the `x/rollup` module, a withdrawal transaction results in a simple ETH burn on the L2, along with emission of corresponding `withdrawal_initiated` and `burn_eth` events.

### Monomer Block Builder

After each L2 appchain block, the Monomer block builder listens for `withdrawal_initiated` events from successful L2 transactions.
For each observed event, the builder makes a corresponding `L2ToL1MessagePasserExecutor` contract call into the internal EVM sidecar state to store the withdrawal transaction for use when proving the withdrawal on L1.

## Proving and Finalizing Withdrawals

The next step for a user is to obtain a proof of the withdrawal transaction.

For compatibility with the withdrawals process, Monomer uses the state root of its EVM sidecar state as the L2 state updated by the `op-proposer`. Monomer exposes the standard ethereum `GetProof` API endpoint for obtaining a merkle proof of withdrawal transactions registered in the EVM sidecar state.

With the withdrawal proof data, the user is now back to the L1 side of the OP Stack. The proof is submitted, and the withdrawal can be finalized after the rollup's challenge period.
