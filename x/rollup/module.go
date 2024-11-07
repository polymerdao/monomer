package rollup

import (
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	modulev1 "github.com/polymerdao/monomer/gen/rollup/module/v1"
	"github.com/polymerdao/monomer/x/rollup/keeper"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ModuleInputs struct {
	depinject.In

	Codec         codec.Codec
	StoreService  store.KVStoreService
	BankKeeper    bankkeeper.Keeper
	AccountKeeper authkeeper.AccountKeeper
}

type ModuleOutputs struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

func init() { //nolint:gochecknoinits
	appmodule.Register(&modulev1.Module{}, appmodule.Provide(ProvideModule), appmodule.Provide(ProvideCustomGetSigner))
}

func ProvideCustomGetSigner() signing.CustomGetSigner {
	return signing.CustomGetSigner{
		// gocosmos is built on gogoproto, which generates protov1 protobufs, unfortunately.
		MsgType: protoreflect.FullName(protov1.MessageName(&types.MsgApplyL1Txs{})), //nolint:staticcheck
		Fn: func(msg proto.Message) ([][]byte, error) {
			return [][]byte{}, nil
		},
	}
}

func ProvideModule(in ModuleInputs) ModuleOutputs { //nolint:gocritic // hugeParam
	k := keeper.NewKeeper(in.Codec, in.StoreService, in.BankKeeper, in.AccountKeeper)
	return ModuleOutputs{
		Keeper: k,
		Module: NewAppModule(in.Codec, k),
	}
}

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// ----------------------------------------------------------------------------
// AppModuleBasic
// ----------------------------------------------------------------------------

// AppModuleBasic implements the AppModuleBasic interface for the rollup module.
type AppModuleBasic struct {
	cdc codec.BinaryCodec
}

func NewAppModuleBasic(cdc codec.BinaryCodec) AppModuleBasic {
	return AppModuleBasic{cdc: cdc}
}

// Name returns the rollup module's name.
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

func (AppModuleBasic) RegisterCodec(_ *codec.LegacyAmino) {
}

func (AppModuleBasic) RegisterLegacyAminoCodec(_ *codec.LegacyAmino) {
}

// RegisterInterfaces registers the module's interface types
func (a AppModuleBasic) RegisterInterfaces(reg cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(reg)
}

// DefaultGenesis returns the rollup module's default genesis state.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesisState())
}

// ValidateGenesis performs genesis state validation for the rollup module.
func (AppModuleBasic) ValidateGenesis(
	cdc codec.JSONCodec,
	_ client.TxEncodingConfig,
	bz json.RawMessage,
) error {
	var data types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &data); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}

	return types.ValidateGenesis(data)
}

// RegisterRESTRoutes registers the rollup module's REST service handlers.
func (AppModuleBasic) RegisterRESTRoutes(clientCtx client.Context, rtr *mux.Router) { //nolint:gocritic
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) { //nolint:gocritic
}

// GetTxCmd returns the rollup module's root tx command.
func (a AppModuleBasic) GetTxCmd() *cobra.Command {
	return nil
}

// GetQueryCmd returns the rollup module's root query command.
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return nil
}

// ----------------------------------------------------------------------------
// AppModule
// ----------------------------------------------------------------------------

// AppModule implements the AppModule interface for the rollup module.
type AppModule struct {
	AppModuleBasic
	keeper *keeper.Keeper
}

func NewAppModule(
	cdc codec.Codec,
	k *keeper.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: NewAppModuleBasic(cdc),
		keeper:         k,
	}
}

func (am AppModule) IsAppModule() {}

func (am AppModule) IsOnePerModuleType() {}

// Name returns the rollup module's name.
func (am AppModule) Name() string {
	return am.AppModuleBasic.Name()
}

// QuerierRoute returns the rollup module's query routing key.
func (AppModule) QuerierRoute() string { return types.QuerierRoute }

// RegisterServices registers a Msg service to respond to module-specific messages and a GRPC query service to respond
// to module-specific GRPC queries.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), am.keeper)
	types.RegisterQueryServer(cfg.QueryServer(), am.keeper)
}

// RegisterInvariants registers the rollup module's invariants.
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// InitGenesis performs genesis initialization for the rollup module.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) { //nolint:gocritic // hugeParam
	var genesisState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)
	am.keeper.InitGenesis(ctx, genesisState)
}

// ExportGenesis returns the rollup module's exported genesis state as raw JSON bytes.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage { //nolint:gocritic
	gs := am.keeper.ExportGenesis(ctx)
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return 2 }
