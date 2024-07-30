---
sidebar_position: 2
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
git checkout v0.50.4
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
    --disabled=cosmwasm,globalfee \
    --org=monomer
```

This gives us a boilerplate Cosmos SDK application in the `rollchain`
directory.

## Integrating Monomer
Let's move into the `rollchain` directory and add Monomer to our application.

```bash
cd rollchain
go get github.com/polymerdao/monomer
```

### Import `x/rollup` and `x/testmodule`

Now we can import [`x/rollup`](/docs/learn/the-rollup-module) in `app/app.go`. While we're at it, let's also import `x/testmodule` (this
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
        &app.MintKeeper,
        app.BankKeeper,
    ),
),
testmodule.New(testmodulekeeper.New(runtime.NewKVStoreService(keys[testmodule.StoreKey]))),
```

### Finalizing our Changes
Near the bottom of the `NewChainApp` function, comment out the following lines:
```go
// app/app.go

/*protoFiles, err := proto.MergedRegistry()
if err != nil {
    panic(err)
}
err = msgservice.ValidateProtoAnnotations(protoFiles)
if err != nil {
    // Once we switch to using protoreflect-based antehandlers, we might
    // want to panic here instead of logging a warning.
    _, _ = fmt.Fprintln(os.Stderr, err.Error())
}*/
```
and remove this import: `github.com/cosmos/cosmos-sdk/types/msgservice`. 



That's it for `app.go`! Next we'll add a CLI command to interact with Monomer.

### Defining the Monomer Command
In `cmd/rolld/commands.go`, import `github.com/polymerdao/monomer/integrations` and add the following block to the `initRootCmd`
function. This defines the `monomer` command which utilizes our custom start
command hook that handles the integration between Monomer and the Cosmos SDK:
```go
// cmd/rolld/commands.go

import (
    // ...
    "github.com/polymerdao/monomer/integrations" // <-- add this line
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
Before we can build our application, we need to add the following replace directive in `go.mod` and then run `go mod
tidy`: 
```go
github.com/ethereum/go-ethereum => github.com/joshklop/op-geth v0.0.0-20240515205036-e3b990384a74
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

This will create a new directory in `~/.rollchain` with the necessary application configuration files in `~/.rollchain/config`. For the sake of this tutorial, we **must tweak the generated `genesis.json` file** slightly.
1. Update the `chain_id` parameter to a numeric value.
2. Include `x/testmodule` in `app_state` so it can be loaded at genesis.

Copy the following genesis file to `~/.rollchain/config/genesis.json` or make the changes directly.

<details>
<summary>`genesis.json`</summary>

```json
{
  "app_name": "rolld",
  "app_version": "cbe7dd3",
  "genesis_time": "2024-07-24T18:51:39.590333Z",
  "chain_id": "1",
  "initial_height": 1,
  "app_hash": null,
  "app_state": {
    "testmodule": {},
    "07-tendermint": null,
    "08-wasm": {
      "contracts": []
    },
    "auth": {
      "params": {
        "max_memo_characters": "256",
        "tx_sig_limit": "7",
        "tx_size_cost_per_byte": "10",
        "sig_verify_cost_ed25519": "590",
        "sig_verify_cost_secp256k1": "1000"
      },
      "accounts": []
    },
    "authz": {
      "authorization": []
    },
    "bank": {
      "params": {
        "send_enabled": [],
        "default_send_enabled": true
      },
      "balances": [],
      "supply": [],
      "denom_metadata": [],
      "send_enabled": []
    },
    "capability": {
      "index": "1",
      "owners": []
    },
    "circuit": {
      "account_permissions": [],
      "disabled_type_urls": []
    },
    "consensus": null,
    "crisis": {
      "constant_fee": {
        "denom": "stake",
        "amount": "1000"
      }
    },
    "distribution": {
      "params": {
        "community_tax": "0.020000000000000000",
        "base_proposer_reward": "0.000000000000000000",
        "bonus_proposer_reward": "0.000000000000000000",
        "withdraw_addr_enabled": true
      },
      "fee_pool": {
        "community_pool": []
      },
      "delegator_withdraw_infos": [],
      "previous_proposer": "",
      "outstanding_rewards": [],
      "validator_accumulated_commissions": [],
      "validator_historical_rewards": [],
      "validator_current_rewards": [],
      "delegator_starting_infos": [],
      "validator_slash_events": []
    },
    "evidence": {
      "evidence": []
    },
    "feegrant": {
      "allowances": []
    },
    "feeibc": {
      "identified_fees": [],
      "fee_enabled_channels": [],
      "registered_payees": [],
      "registered_counterparty_payees": [],
      "forward_relayers": []
    },
    "genutil": {
      "gen_txs": []
    },
    "gov": {
      "starting_proposal_id": "1",
      "deposits": [],
      "votes": [],
      "proposals": [],
      "deposit_params": null,
      "voting_params": null,
      "tally_params": null,
      "params": {
        "min_deposit": [
          {
            "denom": "stake",
            "amount": "10000000"
          }
        ],
        "max_deposit_period": "172800s",
        "voting_period": "172800s",
        "quorum": "0.334000000000000000",
        "threshold": "0.500000000000000000",
        "veto_threshold": "0.334000000000000000",
        "min_initial_deposit_ratio": "0.000000000000000000",
        "proposal_cancel_ratio": "0.500000000000000000",
        "proposal_cancel_dest": "",
        "expedited_voting_period": "86400s",
        "expedited_threshold": "0.667000000000000000",
        "expedited_min_deposit": [
          {
            "denom": "stake",
            "amount": "50000000"
          }
        ],
        "burn_vote_quorum": false,
        "burn_proposal_deposit_prevote": false,
        "burn_vote_veto": true,
        "min_deposit_ratio": "0.010000000000000000"
      },
      "constitution": ""
    },
    "group": {
      "group_seq": "0",
      "groups": [],
      "group_members": [],
      "group_policy_seq": "0",
      "group_policies": [],
      "proposal_seq": "0",
      "proposals": [],
      "votes": []
    },
    "ibc": {
      "client_genesis": {
        "clients": [],
        "clients_consensus": [],
        "clients_metadata": [],
        "params": {
          "allowed_clients": [
            "*"
          ]
        },
        "create_localhost": false,
        "next_client_sequence": "0"
      },
      "connection_genesis": {
        "connections": [],
        "client_connection_paths": [],
        "next_connection_sequence": "0",
        "params": {
          "max_expected_time_per_block": "30000000000"
        }
      },
      "channel_genesis": {
        "channels": [],
        "acknowledgements": [],
        "commitments": [],
        "receipts": [],
        "send_sequences": [],
        "recv_sequences": [],
        "ack_sequences": [],
        "next_channel_sequence": "0",
        "params": {
          "upgrade_timeout": {
            "height": {
              "revision_number": "0",
              "revision_height": "0"
            },
            "timestamp": "600000000000"
          }
        }
      }
    },
    "interchainaccounts": {
      "controller_genesis_state": {
        "active_channels": [],
        "interchain_accounts": [],
        "ports": [],
        "params": {
          "controller_enabled": true
        }
      },
      "host_genesis_state": {
        "active_channels": [],
        "interchain_accounts": [],
        "port": "icahost",
        "params": {
          "host_enabled": true,
          "allow_messages": [
            "*"
          ]
        }
      }
    },
    "mint": {
      "minter": {
        "inflation": "0.130000000000000000",
        "annual_provisions": "0.000000000000000000"
      },
      "params": {
        "mint_denom": "stake",
        "inflation_rate_change": "0.130000000000000000",
        "inflation_max": "0.200000000000000000",
        "inflation_min": "0.070000000000000000",
        "goal_bonded": "0.670000000000000000",
        "blocks_per_year": "6311520"
      }
    },
    "nft": {
      "classes": [],
      "entries": []
    },
    "packetfowardmiddleware": {
      "params": {
        "fee_percentage": "0.000000000000000000"
      },
      "in_flight_packets": {}
    },
    "params": null,
    "poa": {
      "params": {
        "admins": [
          "roll10d07y265gmmuvt4z0w9aw880jnsr700j5y2waw"
        ],
        "allow_validator_self_exit": true
      }
    },
    "ratelimit": {
      "params": {},
      "rate_limits": [],
      "whitelisted_address_pairs": [],
      "blacklisted_denoms": [],
      "pending_send_packet_sequence_numbers": [],
      "hour_epoch": {
        "epoch_number": "0",
        "duration": "3600s",
        "epoch_start_time": "0001-01-01T00:00:00Z",
        "epoch_start_height": "0"
      }
    },
    "slashing": {
      "params": {
        "signed_blocks_window": "100",
        "min_signed_per_window": "0.500000000000000000",
        "downtime_jail_duration": "600s",
        "slash_fraction_double_sign": "0.050000000000000000",
        "slash_fraction_downtime": "0.010000000000000000"
      },
      "signing_infos": [],
      "missed_blocks": []
    },
    "staking": {
      "params": {
        "unbonding_time": "1814400s",
        "max_validators": 100,
        "max_entries": 7,
        "historical_entries": 10000,
        "bond_denom": "stake",
        "min_commission_rate": "0.000000000000000000"
      },
      "last_total_power": "0",
      "last_validator_powers": [],
      "validators": [],
      "delegations": [],
      "unbonding_delegations": [],
      "redelegations": [],
      "exported": false
    },
    "tokenfactory": {
      "params": {
        "denom_creation_fee": [
          {
            "denom": "stake",
            "amount": "10000000"
          }
        ],
        "denom_creation_gas_consume": "2000000"
      },
      "factory_denoms": []
    },
    "transfer": {
      "port_id": "transfer",
      "denom_traces": [],
      "params": {
        "send_enabled": true,
        "receive_enabled": true
      },
      "total_escrowed": []
    },
    "upgrade": {},
    "vesting": {}
  },
  "consensus": {
    "params": {
      "block": {
        "max_bytes": "22020096",
        "max_gas": "-1"
      },
      "evidence": {
        "max_age_num_blocks": "100000",
        "max_age_duration": "172800000000000",
        "max_bytes": "1048576"
      },
      "validator": {
        "pub_key_types": [
          "ed25519"
        ]
      },
      "version": {
        "app": "0"
      },
      "abci": {
        "vote_extensions_enable_height": "0"
      }
    }
  }
}
```
</details>

## Running the Application

Now that our application is configured, we can start the Monomer application by
running

```bash
rolld monomer start
```

Congratulations! You've successfully integrated Monomer into your Cosmos SDK
application. 
