package testmodule

import (
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	crypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/gorilla/mux"
	grpcruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/polymerdao/monomer/gen/testapp/module/v1"
	testappv1 "github.com/polymerdao/monomer/gen/testapp/v1"
	"github.com/polymerdao/monomer/testapp/x/testmodule/keeper"
	"github.com/spf13/cobra"
)

type ModuleInputs struct {
	depinject.In

	StoreService store.KVStoreService
}

type ModuleOutputs struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

func init() {
	appmodule.Register(&modulev1.Module{}, appmodule.Provide(ProvideModule))
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	k := keeper.New(in.StoreService)
	return ModuleOutputs{
		Keeper: k,
		Module: New(k),
	}
}

const (
	ModuleName = "testmodule"
	StoreKey   = ModuleName
)

type Module struct {
	keeper *keeper.Keeper
}

var (
	_ module.AppModule      = (*Module)(nil)
	_ module.HasABCIGenesis = (*Module)(nil)
)

func New(k *keeper.Keeper) *Module {
	return &Module{
		keeper: k,
	}
}

func (m *Module) IsOnePerModuleType() {}

func (m *Module) IsAppModule() {}

func (m *Module) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) []abcitypes.ValidatorUpdate {
	genesis := make(map[string]string)
	if err := json.Unmarshal(data, &genesis); err != nil { // The sdk ensures the data is never nil.
		panic(fmt.Errorf("unmarshal genesis data: %v", err))
	}
	if err := m.keeper.InitGenesis(ctx, genesis); err != nil {
		panic(err)
	}
	return []abcitypes.ValidatorUpdate{
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
func (*Module) RegisterGRPCGatewayRoutes(_ client.Context, _ *grpcruntime.ServeMux) {
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
	testappv1.RegisterSetServiceServer(cfg.MsgServer(), m.keeper)
	testappv1.RegisterGetServiceServer(cfg.QueryServer(), m.keeper)
}
