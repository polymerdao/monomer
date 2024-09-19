---
sidebar_position: 1
---

# Create an Application With Monomer

In this tutorial, you will learn how to create a new Monomer application with
a Cosmos SDK [simapp](https://docs.cosmos.network/main/learn/intro/sdk-design#baseapp) (or, `baseapp`).

## Setup

Monomer is a framework for deploying Cosmos SDK applications as rollups on
Ethereum. As such, the development environment for Monomer is similar to that of
a Cosmos SDK application. To begin building an application with Monomer, we can
start with a basic Cosmos SDK app.

We'll use the handy tool [spawn](https://github.com/rollchains/spawn) to bootstrap a new Cosmos SDK application. If you do not already have it, follow the instructions below to install it.

<details>
  <summary>Install `spawn`</summary>

```bash
git clone https://github.com/rollchains/spawn
cd spawn
git checkout v0.50.5
make install
```

</details>

### Bootstrapping a Cosmos SDK Application

We can spawn a new Cosmos SDK application with `spawn new`:

```bash
spawn new rollchain --consensus=proof-of-authority \
    --bech32=roll \
    --denom=uroll \
    --bin=rolld \
    --disabled=cosmwasm,globalfee,block-explorer \
    --org=monomer
```

This gives us a boilerplate Cosmos SDK application in the `rollchain`
directory.

## Integrating Monomer

Let's move into the `rollchain` directory and add Monomer to our application.

```bash
cd rollchain
```

### Import `x/rollup` and `x/testmodule`

Now we can import [`x/rollup`](/learn/the-rollup-module) in `app/app.go`. While we're at it, let's also import `x/testmodule` (this
will [initialize a non-empty validator set](https://github.com/polymerdao/monomer/blob/c98eccb49bf857829cadee899359e60fc36e6745/testapp/x/testmodule/module.go#L82) for us so we don't have to do it manually). When you're ready to deploy your application, you can remove `x/testmodule` and configure your own validator set. Add the following packages to the import statement in `app/app.go`:

```go
import (
    // ...
    "sync"

    "github.com/polymerdao/monomer/x/rollup"                                     // <-- add this line
    rolluptypes "github.com/polymerdao/monomer/x/rollup/types"                   // <-- add this line
    rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"                 // <-- add this line
    "github.com/polymerdao/monomer/testapp/x/testmodule"                         // <-- add this line
    testmodulekeeper "github.com/polymerdao/monomer/testapp/x/testmodule/keeper" // <-- add this line
    // ...
)
```

and run `go mod tidy`.

There are a few modifications we need to make to the boilerplate Cosmos SDK app.
Namely, we need to

1. Initialize the module's store keys
2. Add the module to the genesis module order
3. Initialize the module in `app/app.go`

Let's walk through these changes.

### Setting the Store Keys

Look for the block that initializes the KV store keys, and add the store key for `x/rollup` and `x/testmodule`:

```go
// app/app.go

func NewChainApp(
  logger log.Logger,
  db dbm.DB,
  traceStore io.Writer,
  loadLatest bool,
  appOpts servertypes.AppOptions,
  baseAppOptions ...func(*baseapp.BaseApp),
) *ChainApp {
    // ...
    keys := storetypes.NewKVStoreKeys(
        // ...
        ratelimittypes.StoreKey,
        // Monomer
        rolluptypes.StoreKey, // <-- add this line
        testmodule.StoreKey,  // <-- add this line
    )
    // ...
}
```

### Genesis Module Order

Next, add `x/rollup` and `x/testmodule` to the genesis module order.
Here we place them at the end of the list:

```go
// app/app.go

genesisModuleOrder := []string{
    // ...
    wasmlctypes.ModuleName,
    ratelimittypes.ModuleName,
    // Monomer
    rolluptypes.ModuleName, // <-- add this line
    testmodule.ModuleName,  // <-- add this line
    // this line is used by starport scaffolding # stargate/app/initGenesis
}
```

### Initializing the Modules

Now let's initialize the modules with their respective keepers. Add the
following block to the `NewChainApp` function:

```go
// app/app.go

// ...
ratelimit.NewAppModule(appCodec, app.RateLimitKeeper),
// Monomer
rollup.NewAppModule( // <-- add this block
    appCodec,
    rollupkeeper.NewKeeper(
        appCodec,
        runtime.NewKVStoreService(keys[rolluptypes.StoreKey]),
        app.BankKeeper,
    ),
),
testmodule.New(testmodulekeeper.New(runtime.NewKVStoreService(keys[testmodule.StoreKey]))),
```

That's it for `app.go`! Next we'll add a CLI command to interact with Monomer.

### Defining the Monomer Command

In `cmd/rolld/commands.go`, import `github.com/polymerdao/monomer/integrations` and add the following block to the `initRootCmd`
function. This defines the `monomer` command which utilizes our custom start
command hook that handles the integration between Monomer and the Cosmos SDK:

```go
// cmd/rolld/commands.go

import (
    // ...
    "github.com/polymerdao/monomer/integrations" // <-- add this line, run `go mod tidy`
    // ...
)

func initRootCmd(
  rootCmd *cobra.Command,
  txConfig client.TxConfig,
  interfaceRegistry codectypes.InterfaceRegistry,
  appCodec codec.Codec,
  basicManager module.BasicManager,
) {
  // ...

  // Remove or comment out the following line:
  /* server.AddCommands(rootCmd, app.DefaultNodeHome, newApp, appExport, addModuleInitFlags) */

  // --- Add this block ---
  monomerCmd := &cobra.Command{
	Use:     "monomer",
	Aliases: []string{"comet", "cometbft", "tendermint"},
	Short:   "Monomer subcommands",
  }
  monomerCmd.AddCommand(server.StartCmdWithOptions(newApp, app.DefaultNodeHome, server.StartCmdOptions{
    StartCommandHandler: integrations.StartCommandHandler,
  }))
  rootCmd.AddCommand(monomerCmd)
  // --- End block ---

  // ...
}
```

## Building the Application

To resolve some breaking changes in minor versions, we need the following replace directives. Add them to `go.mod`, then run `go mod
tidy`.

```go
github.com/ethereum/go-ethereum => github.com/joshklop/op-geth v0.0.0-20240515205036-e3b990384a74
github.com/libp2p/go-libp2p => github.com/joshklop/go-libp2p v0.0.0-20240814165419-c6b91fa9f263
github.com/btcsuite/btcd/btcec/v2 v2.3.4 => github.com/btcsuite/btcd/btcec/v2 v2.3.2
github.com/crate-crypto/go-kzg-4844 v1.0.0 => github.com/crate-crypto/go-kzg-4844 v0.7.0
github.com/crate-crypto/go-ipa => github.com/crate-crypto/go-ipa v0.0.0-20231205143816-408dbffb2041
```

and change the `cosmossdk.io/core` replace directive to `v0.11.1`:

```
cosmossdk.io/core => cosmossdk.io/core v0.11.1
```

Now we can build our application by running

```bash
make install
```

from the root of the
`rollchain/` directory. This will install our chain binary, `rolld`, to the
`$GOBIN` directory.

## Configuring the Application

There's one last step before we can run our application: We need to write
its genesis file. We can generate one by running

```bash
rolld init my-monomer-app --chain-id=1
```

:::important
Make sure to use a numeric chain ID. Monomer will fail to start if the chain ID
cannot be parsed as a `uint64`.
:::

This will create a new directory in `~/.rollchain` with the necessary application configuration files in `~/.rollchain/config`.

## Running the Application

Now that our application is configured, we can start the Monomer application by
running

```bash
rolld monomer start
```

Congratulations! You've successfully integrated Monomer into your Cosmos SDK
application.
