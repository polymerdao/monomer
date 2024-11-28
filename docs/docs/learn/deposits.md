---
sidebar_position: 2
---

import ThemedImage from '@theme/ThemedImage';

# Deposits

Monomer supports ETH deposits through the [`OptimismPortal`](https://github.com/ethereum-optimism/optimism/blob/d48b45954c381f75a13e61312da68d84e9b41418/packages/contracts-bedrock/src/L1/OptimismPortal.sol) L1 contract. This is the canonical way to move assets from Ethereum to the Monomer rollup chain, and the easiest way to bootstrap liquidity on the rollup chain.

:::note
Currently, only `value` deposits are supported. General `transactions` originating from the L1 are a roadmap item.
:::

The deposit transaction flow is illustrated below. Broadly, the lifecycle moves from OP-Stack components, to the Monomer adapter layer, and finally into to the Cosmos-SDK appchain.

<ThemedImage
alt="Deposit workflow"
sources={{ light: '/img/deposits-light.png', dark: '/img/deposits-dark.png' }}
/>

## The OP Stack

The first three steps are entirely the domain of the OP-stack infrastructure. Briefly:

1. An ethereum user sends a deposit transaction to the L1 Optimism Portal contract. This contract is part of the OP Stack core infrastructure for rollups.

The OP stack protocol is obligated to include observed L1 transactions in the L2 blocks it produces within a given timeout.

2. The `op-node` listens for these L1 deposits, and
3. passes them to the execution layer via the `EngineAPI`

Notably, the `op-node` is blind to the fact that it is communicating with Monomer rather than a vanilla EVM execution client (like `op-geth`). Refer to the [OP Stack specification](https://specs.optimism.io/protocol/deposits.html) for more details on the deposit workflow.

## The Monomer Adapter

In order to communicate with the `op-node`, Monomer implements the execution layer endpoints for the [EngineAPI](https://hackmd.io/@danielrachi/engine_api).

Because the OP stack and the EngineAPI are Ethereum specific, Monomer's chief responsibility is to translate the Ethereum encoded deposit transactions into a format that the Cosmos-SDK appchain can understand.

4. `AdaptPayloadTXsToCosmosTxs(...)` is called on each received EngineAPI call with transaction payloads.

In the `x/rollup` module, Monomer defines a custom Cosmos-SDK message type to carry deposit transaction data.

For each rollup block, all deposit transactions sourced from the L1 are batched into a single `DepositTxs` transaction. The engine now awaits the `op-node`'s request to finalize a block. When that request comes:

5. the cached transaction is passed to Monomer's `builder.Build()`, which encapsulates the Cosmos-SDK appchain. In accordance with the OP stack spec, the `DepositTxs` transaction is the first transaction in each L2 block.

## The Cosmos-SDK Appchain

The Cosmos Appchain now builds a block as usual. Monomer's contribution here is the `x/rollup` module, which:

6. unpacks the deposit transaction, minting ETH according to embedded values
7. emits events for each deposit
