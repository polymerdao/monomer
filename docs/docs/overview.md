---
sidebar_position: 2
slug: /overview
---

# Overview

## Advancing interoperability

Ethereum’s rollup-centric roadmap has led to the proliferation of blockchains at Layer 2. Most aim to achieve some level of Ethereum equivalence. While this may be sufficient for many use cases, application-specific blockchains enable developers to deliver vertically-integrated user experiences at lower costs.

The Cosmos SDK is a framework for building application-specific blockchains. Applications built using the Cosmos SDK are composed of modules, many of which are available out-of-the box, allowing developers to easily build their own. For example, the widely-used IBC (Inter-Blockchain Communication) modules facilitate trust-minimized bridging.

By deploying a Cosmos SDK app on Ethereum, developers get the best of both worlds: direct access to Ethereum’s vast liquidity and user base, coupled with state-of-the-art app chain tooling in the Cosmos SDK.

## Deploying Cosmos SDK applications on the OP stack

The OP Stack is a set of independent components used to build rollups on Ethereum. Broadly speaking, the components are separated into consensus and execution layers.
The consensus layer components control the execution layer’s fork choice via the Engine API.

From a user’s perspective, the execution layer is the interesting part.
Many OP Stack rollups, such as Optimism and Base, use OP-Geth application as their execution layer.
OP-Geth is a slightly modified version of go-ethereum, an Ethereum execution layer client.

:::note[OP Stack Flow]
![OP Stack Flow](/docs/static/img/op-stack.png)
:::

Currently, most Cosmos SDK applications are deployed on top of CometBFT, an implementation of the Tendermint consensus algorithm.
Unlike the OP Stack which uses the Engine API, CometBFT sends fork choice updates to the application using the Application BlockChain Interface (ABCI).

:::note[Cosmos SDK Flow]
![Cosmos SDK Flow](/docs/static/img/cosmos-abci.png)
:::

Monomer allows us to take any Cosmos SDK application and deploy it as the execution layer on the OP stack.
It can be thought of as a translator between the consensus layer that speaks the Engine API and the execution layer that understands the ABCI.

## Architecture at a Glance

From the [OP stack](https://specs.optimism.io/protocol/overview.html#components)'s perspective, Monomer replaces the default Ethereum
compatible execution engine. From the [Cosmos application](https://docs.cosmos.network/v0.50/learn/intro/why-app-specific#what-are-application-specific-blockchains)'s perspective,
Monomer replaces the CometBFT consensus layer.

In order to achieve this, Monomer performs three primary tasks:

- It translates between Ethereum's `EngineAPI` and the Cosmos `ABCI` standards for `consensus<>execution` communication
- It provides a custom Cosmos SDK module for handling rollup-specific logic and state
- It defines a hybridized block head structure, and build process, where the Cosmos AppHash is stored as data in an EVM state tree

:::note[Architecture]
![Architecture](/docs/static/img/architecture.png)
:::

## Why Monomer Matters

Cosmos is recognized for its advanced technology, ready for broader adoption among developers. Monomer is designed to extend the reach of Cosmos technology to a wider developer and user base.

Key Features of Monomer:

- Cosmos SDK and ABCI Compatibility
  - Monomer allows any Cosmos SDK or ABCI-compatible app chain to be deployed as a rollup on Ethereum.
- Customization and Flexibility
  - Unlike most Ethereum rollup frameworks that focus on EVM compatibility, Monomer supports extensive customization options through the Cosmos SDK, including swapping out the underlying key-value store and state commitment data structures.
- Advanced Developer Support
  - The Cosmos tech stack is backed by extensive documentation and resources, making it easier for developers to adopt and innovate.
- Innovation Import
  - Monomer facilitates the integration of unique Cosmos innovations into the Ethereum ecosystem, such as Skip’s Block SDK, enhancing the technological toolbox available to Ethereum developers.
- Diverse Virtual Machine Support
  - Developers can deploy rollups using different types of VMs, including EVM and CosmWasm, and write app-specific logic in Go without relying on a VM.

## Monomer/CometBFT RPC Compatibility

|                                                                                              | CometBFT | Monomer |
|----------------------------------------------------------------------------------------------|:--------:|:-------:|
| [**/health**](https://docs.cometbft.com/v0.34/rpc/#/Info/health)                             |    ✅     |    ✅    |
| [**/status**](https://docs.cometbft.com/v0.34/rpc/#/Info/status)                             |    ✅     |    ✅    |
| [**/net_info**](https://docs.cometbft.com/v0.34/rpc/#/Info/net_info)                         |    ✅     |    ❌    |
| [**/blockchain**](https://docs.cometbft.com/v0.34/rpc/#/Info/blockchain)                     |    ✅     |    ❌    |
| [**/block**](https://docs.cometbft.com/v0.34/rpc/#/Info/block)                               |    ✅     |    ✅    |
| [**/block_by_hash**](https://docs.cometbft.com/v0.34/rpc/#/Info/block_by_hash)               |    ✅     |    ✅    |
| [**/block_results**](https://docs.cometbft.com/v0.34/rpc/#/Info/block_results)               |    ✅     |    ❌    |
| [**/commit**](https://docs.cometbft.com/v0.34/rpc/#/Info/commit)                             |    ✅     |    ❌    |
| [**/validators**](https://docs.cometbft.com/v0.34/rpc/#/Info/validators)                     |    ✅     |    ❌    |
| [**/genesis**](https://docs.cometbft.com/v0.34/rpc/#/Info/genesis)                           |    ✅     |    ❌    |
| [**/genesis_chunked**](https://docs.cometbft.com/v0.34/rpc/#/Info/genesis_chunked)           |    ✅     |    ❌    |
| [**/dump_consensus_state**](https://docs.cometbft.com/v0.34/rpc/#/Info/dump_consensus_state) |    ✅     |    ❌    |
| [**/consensus_state**](https://docs.cometbft.com/v0.34/rpc/#/Info/consensus_state)           |    ✅     |    ❌    |
| [**/unconfirmed_txs**](https://docs.cometbft.com/v0.34/rpc/#/Info/unconfirmed_txs)           |    ✅     |    ❌    |
| [**/num_unconfirmed_txs**](https://docs.cometbft.com/v0.34/rpc/#/Info/num_unconfirmed_txs)   |    ✅     |    ❌    |
| [**/tx_search**](https://docs.cometbft.com/v0.34/rpc/#/Info/tx_search)                       |    ✅     |    ✅    |
| [**/block_search**](https://docs.cometbft.com/v0.34/rpc/#/Info/block_search)                 |    ✅     |    ❌    |
| [**/tx**](https://docs.cometbft.com/v0.34/rpc/#/Info/tx)                                     |    ✅     |    ✅    |
| [**/broadcast_evidence**](https://docs.cometbft.com/v0.34/rpc/#/Info/broadcast_evidence)     |    ✅     |    ❌    |
| [**/broadcast_tx_sync**](https://docs.cometbft.com/v0.34/rpc/#/Tx/broadcast_tx_sync)         |    ✅     |    ✅    |
| [**/broadcast_tx_async**](https://docs.cometbft.com/v0.34/rpc/#/Tx/broadcast_tx_async)       |    ✅     |    ✅    |
| [**/broadcast_tx_commit**](https://docs.cometbft.com/v0.34/rpc/#/Tx/broadcast_tx_commit)     |    ✅     |    ❌    |
| [**/check_tx**](https://docs.cometbft.com/v0.34/rpc/#/Tx/check_tx)                           |    ✅     |    ❌    |
| [**/abci_query**](https://docs.cometbft.com/v0.34/rpc/#/ABCI/abci_query)                     |    ✅     |    ✅    |
| [**/abci_info**](https://docs.cometbft.com/v0.34/rpc/#/ABCI/abci_info)                       |    ✅     |    ✅    |
| [**/dial_seeds**](https://docs.cometbft.com/v0.34/rpc/#/Unsafe/dial_seeds)                   |    ✅     |    ❌    |
| [**/dial_peers**](https://docs.cometbft.com/v0.34/rpc/#/Unsafe/dial_peers)                   |    ✅     |    ❌    |
| [**/subscribe (Websocket)**](https://docs.cometbft.com/v0.34/rpc)                            |    ✅     |    ✅    |
| [**/unsubscribe (Websocket)**](https://docs.cometbft.com/v0.34/rpc)                          |    ✅     |    ✅    |
| [**/unsubscribe_all (Websocket)**](https://docs.cometbft.com/v0.34/rpc)                      |    ✅     |    ✅    |
