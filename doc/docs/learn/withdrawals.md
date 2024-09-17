---
sidebar_position: 4
---

import ThemedImage from '@theme/ThemedImage';

# Withdrawals

Monomer supports ETH withdrawals through the Optimism Portal L1 contract. This is the canonical way to move assets from an OP Stack rollup chain to Ethereum.

:::note
For UX purposes, exiting an ORU via the canonical bridge is impractical and slow. But the exit mechanism must be available for security reasons.
:::

The withdrawal transaction flow is illustrated below. The lifecycle moves in the opposite direction than the Deposits flow - starting at the Cosmos-SDK appchain, processed by the Monomer `x/rollup` module and then Monomer's builder, and finally into the OP-Stack components on L1.

Again, this document focuses on Monomer specific modifiactions. Full documentation for the withdrawal flow is available in the [OP Stack specs](https://specs.optimism.io/protocol/withdrawals.html).

<ThemedImage
alt="Withdrawal workflow"
sources={{ light: '/img/withdrawals-light.png', dark: '/img/withdrawals-dark.png' }}
/>

## Initiating Withdrawals

The withdrawal process begins with an L2 user submitting a withdrawal transaction to the Cosmos-SDK appchain, with the same data used to initiate a withdrawal on a standard OP Stack. These messages are interpreted by both the `x/rollup` module of the appchain, and the Monomer block builder, which writes necessary data to the [EVM sidecar](./blocks.md#state).

### AppChain

On the AppChain, via custom message types and handlers from the `x/rollup` module, a withdrawal transaction results in a simple ETH burn on the L2, along with emission of corresponding `WithdrawalInitiated` and `BurnEth` events.

### Monomer.Builder

After each AppChain block, the builder listens for `WithdrawalInitiated` events. For each observed event, the builder makes a corresponding `L2ToL1MessagePasserExecuter` contract call into the EVM sidecar state.

## Finalizing Withdrawals

The next step for a user is to obtain a proof of the withdrawal transaction.

For compatability with the withdrawals process, Monomer uses the state root of its EVM sidecar as the L2 state updated by the `op-proposer`. Monomer exposes the standard ethereum `GetProof` API endpoint for obtaining a merkle proof of withdrawal transactions registered in the EVM sidecar.

With the withdrawal proof data, the user is now back to the L1 side of the OP Stack. The proof is submitted, and the withdrawal can be finalized after the rollup's challenge period.
