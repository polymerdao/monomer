package testmodule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
	crypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
	"github.com/spf13/cobra"
)

const (
	ModuleName = "testmodule"
	StoreKey = ModuleName
)

type Module struct {
	key storetypes.StoreKey
	testappv1.UnimplementedSetServiceServer
	testappv1.UnimplementedGetServiceServer
}

var (
	_ module.AppModule  = (*Module)(nil)
	_ module.HasGenesis = (*Module)(nil)
)

func New(key *storetypes.KVStoreKey) *Module {
	return &Module{
		key: key,
	}
}

func (m *Module) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	genesis := make(map[string]string)
	if err := json.Unmarshal(data, &genesis); err != nil { // The sdk ensures the data is never nil.
		panic(fmt.Errorf("unmarshal genesis data: %v", err))
	}
	store := ctx.KVStore(m.key)
	for k, v := range genesis {
		store.Set([]byte(k), []byte(v))
	}
	return []abci.ValidatorUpdate{
		{
			PubKey: crypto.PublicKey{},
			Power:  sdk.DefaultPowerReduction.Int64(),
		},
	}
}

func (*Module) ExportGenesis(_ sdk.Context, _ codec.JSONCodec) json.RawMessage {
	return json.RawMessage("{}")
}

func (*Module) DefaultGenesis(_ codec.JSONCodec) json.RawMessage {
	return json.RawMessage("{}")
}

func (m *Module) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, data json.RawMessage) error {
	genesis := make(map[string]string)
	if err := json.Unmarshal(data, &genesis); err != nil {
		return fmt.Errorf("unmarshal genesis data: %v", err)
	}
	return nil
}

func (m *Module) Get(ctx context.Context, req *testappv1.GetRequest) (*testappv1.GetResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	return &testappv1.GetResponse{
		Value: string(sdkCtx.KVStore(m.key).Get([]byte(key))),
	}, nil
}

func (m *Module) Set(ctx context.Context, req *testappv1.SetRequest) (*testappv1.SetResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	sdkCtx.KVStore(m.key).Set([]byte(key), []byte(req.GetValue()))
	return &testappv1.SetResponse{}, nil
}

func (*Module) GetQueryCmd() *cobra.Command {
	return nil
}

func (*Module) GetTxCmd() *cobra.Command {
	return nil
}

func (*Module) Name() string {
	return ModuleName
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the module.
func (*Module) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {
}

// RegisterRESTRoutes registers the capability module's REST service handlers.
func (*Module) RegisterRESTRoutes(clientCtx client.Context, rtr *mux.Router) {
}

// RegisterInterfaces registers the module's interface types
func (*Module) RegisterInterfaces(r codectypes.InterfaceRegistry) {
	testappv1.RegisterInterfaces(r)
}

func (*Module) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

func (m *Module) RegisterServices(cfg module.Configurator) {
	testappv1.RegisterSetServiceServer(cfg.MsgServer(), m)
	testappv1.RegisterGetServiceServer(cfg.QueryServer(), m)
}
