---
sidebar_position: 3
sidebar_label: L2 Transactions
---

import ThemedImage from '@theme/ThemedImage';

# The Rollup Transaction Mempool

Monomer exposes the standard Cosmos [BroadcastTX](https://docs.cosmos.network/api#tag/Service/operation/BroadcastTx) API endpoint for submitting [cometbft transactions](https://pkg.go.dev/github.com/cometbft/cometbft/types#Tx) directly to the rollup chain.

These transactions are stored in a simple mempool, and included in blocks on a first-come-first-serve basis.

:::note
There are no modifications to the standard cometbft transaction format. This means that any client that can construct a cometbft transaction can interact with the Monomer rollup chain.
:::

<ThemedImage
alt="Deposit workflow"
sources={{ light: '/img/l2-light.png', dark: '/img/l2-dark.png' }}
/>

1. A client submits a cometbft transaction to Monomer's `BroadcastTx` endpoint.
2. Monomer runs some basic validation on the transaction, and if it passes, adds it to the mempool.
3. Sometime later, the `op-node` initiates the production of a new block via the `EngineAPI`.
4. The engine requests a new block from the `builder`.
5. The `builder` fetches transactions from the mempool, and
6. constructs, via the AppChain, a new block.

:::warning
The builder does not currently implement any gas metering logic. This means that blocks can be arbitrarily large.
:::
