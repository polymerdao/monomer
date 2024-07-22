---
sidebar_position: 1
---

# Create an App with Monomer

In this tutorial, you will learn how to create a new Monomer application with
a Cosmos SDK [simapp](https://docs.cosmos.network/main/learn/intro/sdk-design#baseapp) (or, `baseapp`).

## Setup

### Installing `spawn`
We'll use the handy tool [spawn](https://github.com/rollchains/spawn), from the
[Rollchains](https://rollchains.com) team to bootstrap a new application. Let's
install it:
```bash
git clone https://github.com/rollchains/spawn
cd spawn
git checkout v0.50.4
make install
```

### Spawning a New Application
Now that we have `spawn` installed, let's create a new Cosmos SDK
application:
```bash
spawn new rollchain --consensus=proof-of-authority \
    --bech32=roll \
    --denom=uroll \
    --bin=rolld \
    --disabled=cosmwasm,globalfee \
    --org=monomer
```

We now have a boilerplate Cosmos SDK application in the `rollchain`
directory.

## Integrating Monomer into Cosmos SDK Apps is Simple

### Importing the `rollup` Module
Let's import Monomer into our application:
```bash
cd rollchains
go get github.com/polymerdao/monomer
```

Now we can import the `rollup` module in `app/app.go`:
```go
import (
    // ...
    "sync"

    "github.com/polymerdao/monomer/x/rollup"
    rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
    rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
    // ...
)
```
The following modifications need to be made to `app/app.go` to
initialize the `rollup` module, and finalize the
integration of Monomer into the Cosmos SDK application.

### Set the Store Key
Look for the block that initializes the KV store keys, and add the store key for the `rollup` module:
```go
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
        // Monomer
        rolluptypes.StoreKey, // <-- add this line
    )
    // ...
}
```

### Initialize the Module's Genesis
```go
genesisModuleOrder := []string{
    // ...
    wasmlctypes.ModuleName,
    ratelimittypes.ModuleName,
    // Monomer
    rolluptypes.ModuleName, // <-- add this line
    // this line is used by starport scaffolding # stargate/app/initGenesis
}
```

### Initialize the `rollup` Module
```go
// ...
ratelimit.NewAppModule(appCodec, app.RateLimitKeeper),
rollup.NewAppModule( // <-- add this block
    appCodec,
    rollupkeeper.NewKeeper(
        appCodec,
        runtime.NewKVStoreService(keys[rolluptypes.StoreKey]),
        &app.MintKeeper,
        app.BankKeeper,
    ),
),
```

### Defining the Monomer Command
In `cmd/rolld/commands.go`, add the following block to the `initRootCmd`
function. This defines the `monomer` command which utilizes our custom start
command hook that handles the integration between Monomer and the Cosmos SDK:
```go
func initRootCmd(
  rootCmd *cobra.Command,
  txConfig client.TxConfig,
  interfaceRegistry codectypes.InterfaceRegistry,
  appCodec codec.Codec,
  basicManager module.BasicManager,
) {
  // ...
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
}
```
