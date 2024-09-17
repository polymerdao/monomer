---
sidebar_position: 1
slug: /
---

# Overview

Monomer uses the OP stack to make Cosmos SDK applications deployable as
Ethereum rollups.

:::warning
Monomer is pre-production software and should be used with caution.
:::

## At a Glance

From the [OP stack](https://specs.optimism.io/protocol/overview.html#components)'s perspective, Monomer replaces the default Ethereum
compatible execution engine. From the [Cosmos application](https://docs.cosmos.network/v0.50/learn/intro/why-app-specific#what-are-application-specific-blockchains)'s perspective,
Monomer replaces the CometBFT consensus layer.

In order to achieve this, Monomer performs three primary tasks:

- It translates between Ethereum's `EngineAPI` and the Cosmos `ABCI` standards for `consensus<>execution` communication
- It provides a custom Cosmos SDK module for handling rollup-specific logic and state
- It defines a hybridized block head structure, and build process, where the Cosmos AppHash is stored as data in an EVM state tree

:::note[Architecture]
![Architecture](/img/architecture.png)
:::

## This Documentation

The [Learn](./category/learn) section walks through specific data flows in Monomer, describing internal components along the way.

The [Build](./category/build) section provides a tutorial on how to build an application with Monomer.
