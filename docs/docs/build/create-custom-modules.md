---
sidebar_position: 2
---

# Add Custom Modules to your Application

Monomer applications are built on the Cosmos SDK and can therefore be extended with custom modules.
This page will walk you through the process of adding a custom module to your Monomer application.
This tutorial assumes that you already followed the [Create an Application With Monomer](/build/create-an-app-with-monomer) tutorial and have a basic Monomer application set up.

## Creating a Custom Module from Scratch

In this example, we will create a custom module called `x/kvstore` that will be added to the Monomer application.
This module will provide a simple key-value store implementation for setting and getting values from the L2 application.

We will be following the recommended module structure for the [Cosmos SDK](https://docs.cosmos.network/v0.50/build/building-modules/structure#structure).

For this example, we will assume that your application is using the default configuration and is named `testapp`.
If you're using a different module name, please replace all occurrences of `testapp` with your application name.

### Create the Module Directory Structure

Navigate to your application's root directory and create a new directory for the module and for the module protobuf files:

```bash
mkdir -p x/kvstore/keeper
mkdir -p x/kvstore/types
mkdir -p proto/testapp/kvstore/v1
```

### Create the Necessary Protobuf Files

We'll now create the required protobuf files for the module.

The first protobuf file we'll create is the `module.proto` file in the `proto/testapp/kvstore/v1` directory:

```bash
touch proto/testapp/kvstore/v1/module.proto
```

Then, add the following content to the `module.proto` file:

```protobuf
syntax = "proto3";

package testapp.kvstore.module.v1;

option go_package = "github.com/testapp/kvstore";

import "cosmos/app/v1alpha1/module.proto";

// Module is the app config object of the module.
// Learn more: https://docs.cosmos.network/main/building-modules/depinject
message Module {
  option (cosmos.app.v1alpha1.module) = {
    go_import : "github.com/testapp/kvstore"
  };

  // authority defines the custom module authority.
  // if not set, defaults to the governance module.
  string authority = 1;
}
```

Next, we'll create the `tx.proto` and `query.proto` files to store the transaction messages and queries:

```bash
touch proto/testapp/kvstore/v1/tx.proto
touch proto/testapp/kvstore/v1/query.proto
```

Then, add the following content to the `tx.proto` file:

```protobuf
syntax = "proto3";
package testapp.kvstore.v1;

option go_package = "github.com/testapp/kvstore";

import "google/api/annotations.proto";
import "cosmos/query/v1/query.proto";
import "gogoproto/gogo.proto";
import "cosmos_proto/cosmos.proto";

// Msg defines all tx endpoints for the x/kvstore module.
service Msg {
  option (cosmos.msg.v1.service) = true;
  // SetValue defines a method for setting a key-value pair.
  rpc SetValue(MsgSetValue) returns (MsgSetValueResponse) {}
}

// MsgSetValue defines the Msg/SetValue request type for setting a key-value pair.
message MsgSetValue {
  option (cosmos.msg.v1.signer) = "from_address";

  string from_address = 1;
  string key = 2;
  string value = 3;
}

// MsgSetValueResponse defines the Msg/SetValue response type.
message MsgSetValueResponse {}
```

And the following content to the `query.proto` file:

```protobuf
syntax = "proto3";
package testapp.kvstore.v1;

option go_package = "github.com/testapp/kvstore";

import "google/api/annotations.proto";
import "cosmos/query/v1/query.proto";
import "gogoproto/gogo.proto";
import "cosmos_proto/cosmos.proto";

// Query defines the module Query service.
// Query defines the gRPC querier service.
service Query {
  // Value queries a value stored at a given key.
  rpc Value(QueryValueRequest) returns (QueryValueResponse) {}
  option (cosmos.query.v1.module_query_safe) = true;
  option (google.api.http).get =
      "/testapp/kvstore/v1/get/{value}";
}

// QueryValueRequest is the request type for the Query/Value method.
message QueryValueRequest {
  string key = 1;
}

// QueryValueResponse is the response type for the Query/Value method.
message QueryValueResponse {
  string value = 1;
}
```

### Compile the Protobuf Files

Now that we have created the protobuf files, we need to compile them.
Run the following command to compile the protobuf files:

```bash
TODO
```

### Add the Module Types

Next, we'll add the `codec.go` and `msgs.go` files to the `types` directory to specify the module types:

```bash
touch x/kvstore/types/codec.go
touch x/kvstore/types/msgs.go
```

Then, update the `codec.go` file with the following content:

```go
package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
```

Next, update the `msgs.go` file with the following content:

```go
package types

import sdktypes "github.com/cosmos/cosmos-sdk/types"

var _ sdktypes.Msg = (*MsgSetValue)(nil)

func (m *MsgSetValue) ValidateBasic() error {
	return nil
}
```

### Add the Module Keeper

Next, we'll add the `keeper.go` file to the `keeper` directory.
This file will contain the module keeper implementation:

```bash
touch x/kvstore/keeper/keeper.go
```

Then, update the `keeper.go` file with the following content:

```go
package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/core/store"
	"github.com/{your_org}/{your_project}}/x/kvstore/types"
)

type Keeper struct {
	storeService store.KVStoreService
}

func New(storeService store.KVStoreService) *Keeper {
	return &Keeper{
		storeService: storeService,
	}
}

func (m *Keeper) InitGenesis(ctx context.Context, kvs map[string]string) error {
	store := m.storeService.OpenKVStore(ctx)
	for k, v := range kvs {
		if err := store.Set([]byte(k), []byte(v)); err != nil {
			return fmt.Errorf("set: %v", err)
		}
	}
	return nil
}

func (m *Keeper) Value(ctx context.Context, req *types.QueryValueRequest) (*types.QueryValueResponse, error) {
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	valueBytes, err := m.storeService.OpenKVStore(ctx).Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	}
	var value string
	if valueBytes == nil {
		value = ""
	} else {
		value = string(valueBytes)
	}
	return &types.QueryValueResponse{
		Value: value,
	}, nil
}

func (m *Keeper) SetValue(ctx context.Context, req *types.MsgSetValue) (*types.MsgSetValueResponse, error) {
	key := req.GetKey()
	if err := m.storeService.OpenKVStore(ctx).Set([]byte(key), []byte(req.GetValue())); err != nil {
		return nil, fmt.Errorf("set: %v", err)
	}
	return &types.MsgSetValueResponse{}, nil
}
```

### Create the Module Implementation

Next, we'll add the `module.go` file to the `x/kvstore` directory.
This file will contain the module implementation:

```bash
touch x/kvstore/module.go
```

Then, update the `module.go` file with the following content:

```go
package module

import (
	"context"
	"encoding/json"
	"fmt"

	appmodulev2 "cosmossdk.io/core/appmodule/v2"
	"cosmossdk.io/core/registry"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/testapp/kvstore"
	"github.com/testapp/kvstore/keeper"
)

var (
	_ appmodulev2.HasGenesis            = AppModule{}
	_ appmodulev2.AppModule             = AppModule{}
	_ appmodulev2.HasRegisterInterfaces = AppModule{}
	_ appmodulev2.HasConsensusVersion   = AppModule{}
	_ appmodulev2.HasMigrations         = AppModule{}
)

// ConsensusVersion defines the current module consensus version.
const ConsensusVersion = 1

type AppModule struct {
	cdc    codec.Codec
	keeper keeper.Keeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(cdc codec.Codec, keeper keeper.Keeper) AppModule {
	return AppModule{
		cdc:    cdc,
		keeper: keeper,
	}
}

// RegisterLegacyAminoCodec registers the example module's types on the LegacyAmino codec.
func (AppModule) RegisterLegacyAminoCodec(registry.AminoRegistrar) {}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the example module.
func (AppModule) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *gwruntime.ServeMux) {
	if err := example.RegisterQueryHandlerClient(context.Background(), mux, example.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}

// RegisterInterfaces registers interfaces and implementations of the example module.
func (AppModule) RegisterInterfaces(registrar registry.InterfaceRegistrar) {
	example.RegisterInterfaces(registrar)
}

// ConsensusVersion implements AppModule/ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

// RegisterServices registers a gRPC query service to respond to the module-specific gRPC queries.
func (am AppModule) RegisterServices(registrar grpc.ServiceRegistrar) {
	example.RegisterMsgServer(registrar, keeper.NewMsgServerImpl(am.keeper))
	example.RegisterQueryServer(registrar, keeper.NewQueryServerImpl(am.keeper))
}

// RegisterMigrations registers in place module state migration migrations
func (am AppModule) RegisterMigrations(mr appmodulev2.MigrationRegistrar) error {
	// m := keeper.NewMigrator(am.keeper)
	// if err := mr.Register(example.ModuleName, 1, m.Migrate1to2); err != nil {
	// 	return fmt.Errorf("failed to migrate x/%s from version 1 to 2: %v", example.ModuleName, err)
	// }

	return nil
}

// DefaultGenesis returns default genesis state as raw bytes for the module.
func (am AppModule) DefaultGenesis() json.RawMessage {
	return am.cdc.MustMarshalJSON(example.NewGenesisState())
}

// ValidateGenesis performs genesis state validation for the circuit module.
func (am AppModule) ValidateGenesis(bz json.RawMessage) error {
	var data example.GenesisState
	if err := am.cdc.UnmarshalJSON(bz, &data); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", example.ModuleName, err)
	}

	return data.Validate()
}

// InitGenesis performs genesis initialization for the example module.
// It returns no validator updates.
func (am AppModule) InitGenesis(ctx context.Context, data json.RawMessage) error {
	var genesisState example.GenesisState
	if err := am.cdc.UnmarshalJSON(data, &genesisState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", example.ModuleName, err)
	}

	if err := am.keeper.InitGenesis(ctx, &genesisState); err != nil {
		return fmt.Errorf("failed to initialize %s genesis state: %v", example.ModuleName, err)
	}

	return nil
}

// ExportGenesis returns the exported genesis state as raw bytes for the circuit
// module.
func (am AppModule) ExportGenesis(ctx context.Context) (json.RawMessage, error) {
	gs, err := am.keeper.ExportGenesis(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to export %s genesis state: %v", example.ModuleName, err)
	}

	return am.cdc.MarshalJSON(gs)
}
```

Congratulations! You have successfully created a custom module for your Monomer application.
You can now interact with the module on your L2 application.

For a more detailed guide on creating custom modules, refer to the [Cosmos SDK Module Tutorial](https://tutorials.cosmos.network/hands-on-exercise/0-native/2-build-module.html).

## Importing Existing Modules

Existing Cosmos SDK modules can be imported directly into your Monomer application as well.

TODO: Add instructions for importing existing modules (time permitting).
