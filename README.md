# Monomer

Monomer uses the OP stack to make Cosmos applications deployable as Ethereum rollups.

> ⏸️ **Development Status:** Active development on Monomer is currently paused.

> ⚠ Monomer is pre-production software and should be used with caution. ⚠

## At a Glance

![Architecture](./architecture.png)

From the [OP stack](https://specs.optimism.io/protocol/overview.html#components)'s perspective, Monomer replaces the default Ethereum compatible execution engine. From the [Cosmos application](https://docs.cosmos.network/v0.50/learn/intro/why-app-specific#what-are-application-specific-blockchains)'s perspective, Monomer replaces the CometBFT consensus layer.

## Development

We use Go 1.22. We use [`buf`](https://buf.build/) to manage protobufs.

### Prerequisites

1. Install [go](https://go.dev/) 1.22 or higher.
1. Install [jq](https://jqlang.github.io/jq/download/)
1. Install [foundry](https://book.getfoundry.sh/getting-started/installation)
1. Install buf:
   ```sh
   make install-buf
   ```
1. Install golangci-lint:

   ```sh
   make install-golangci-lint

   ```

1. Install go-test-coverage:

   ```sh
   make install-go-test-coverage

   ```

### Running tests

1. Set up the environment for end-to-end (e2e) tests:
   ```sh
   make setup-e2e
   ```
1. Run the e2e tests:
   ```sh
   make e2e
   ```
1. Run the unit tests:
   ```sh
   make test
   ```

### Code Quality, Linting and Coverage

1. Run linting:
   ```sh
   make lint
   ```
1. Check test coverage
   ```sh
   make check-cover
   ```

### Cleaning Up

1. Clean up generated files and artifacts:
   ```sh
   make clean
   ```
