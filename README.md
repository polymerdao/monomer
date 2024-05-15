# Monomer

Monomer uses the OP stack to make Cosmos applications deployable as Ethereum rollups.

> ⚠ Monomer is pre-production software and should be used with caution. ⚠

## At a Glance

![Architecture](./architecture.png)

From the [OP stack](https://specs.optimism.io/protocol/overview.html#components)'s perspective, Monomer replaces the default Ethereum compatible execution engine. From the [Cosmos application](https://docs.cosmos.network/v0.50/learn/intro/why-app-specific#what-are-application-specific-blockchains)'s perspective, Monomer replaces the CometBFT consensus layer.

## Development

We use Go 1.22. To run the e2e tests, you'll need the `foundry` tools and the submodules (`git submodule update --init --recursive`). We use [`buf`](https://buf.build/) to manage protobufs.

The Makefile includes commands for running tests and generating coverage profiles.
