#!/usr/bin/env bash

# Stop execution upon any command failure.
set -e

# Build the app.
# Because we transitively depend on github.com/fjl/memsize, we need to disable checklinkname in go1.23.0 and higher.
goVersion=$(go env GOVERSION)
[[ $goVersion > go1.23.0 || $goVersion == go1.23.0 ]] && ldflags="-ldflags=-checklinkname=0"
go build $ldflags -o testappd ./cmd/testappd

# The following process is identical for a standard Cosmos SDK chain.
# The Cosmos SDK documentation has more information in addition to what is presented here:
#   - https://docs.cosmos.network/main/user/run-node/keyring#adding-keys-to-the-keyring
#   - https://docs.cosmos.network/main/user/run-node/run-node#adding-genesis-accounts

# Initialize the application's config and data directories.
# The chain-id must be numeric as required by the OP Stack.
./testappd init my-app --chain-id 1

# The Cosmos SDK requires at least one validator.
# We will use a dummy account representing the sequencer.
./testappd keys add dummy-account --keyring-backend test

address=$(./testappd keys show dummy-account -a --keyring-backend test)

# Fund the dummy account at genesis.
./testappd genesis add-genesis-account $address 100000000000stake

# Make the dummy account self-delegate as a validator.
./testappd genesis gentx dummy-account 1000000000stake --chain-id 1 --keyring-backend test

# Add the gentx to the genesis file.
./testappd genesis collect-gentxs

# The testapp is ready to run with:
# ```
# ./testappd monomer start --minimum-gas-prices 0.01stake
# ```
# (the input to minimum-gas-prices is configurable).
