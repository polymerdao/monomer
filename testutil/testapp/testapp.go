package testapp

import (
	"encoding/json"
	"fmt"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/polymerdao/monomer/testutil/testapp/x/testmodule"
	"github.com/polymerdao/monomer/x/rollup"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

// App is an app with the absolute minimum amount of configuration required to have the Monomer rollup module.
// It also has a dummy test module for easy transaction testing.
// The test module will initialize a single validator to satisfy the module manager's InitChain invariant that the validator set must be non-empty
// (the requirement doesn't make sense to me since that's a consensus-layer concern).
type App struct {
	*baseapp.BaseApp
	basicManager module.BasicManager
	codec        codec.Codec
}

func (a *App) RollbackToHeight(targetHeight uint64) error {
	return a.CommitMultiStore().RollbackToVersion(int64(targetHeight))
}

// module account permissions
var maccPerms = map[string][]string{
	authtypes.FeeCollectorName:     nil,
	minttypes.ModuleName:           {authtypes.Minter},
	stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
	stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
}

var accountAddressPrefix = "testapp"

func New(appdb dbm.DB, chainID string, logger log.Logger) *App {
	interfaceRegistry := codectypes.NewInterfaceRegistry()

	basicManager := module.NewBasicManager(
		auth.AppModuleBasic{},
		bank.AppModuleBasic{},
		staking.AppModuleBasic{},
		mint.AppModuleBasic{},
		rollup.AppModuleBasic{},
		&testmodule.Module{},
	)
	basicManager.RegisterInterfaces(interfaceRegistry)

	appCodec := codec.NewProtoCodec(interfaceRegistry)
	ba := baseapp.NewBaseApp(
		"testapp",
		log.NewNopLogger(),
		appdb,
		tx.DefaultTxDecoder(appCodec),
		baseapp.SetChainID(chainID),
	)
	ba.SetInterfaceRegistry(interfaceRegistry)

	keys := sdk.NewKVStoreKeys(
		authtypes.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		minttypes.StoreKey,
		rolluptypes.StoreKey,
		testmodule.StoreKey,
	)

	govAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	accountKeeper := authkeeper.NewAccountKeeper(
		appCodec,
		keys[authtypes.StoreKey],
		authtypes.ProtoBaseAccount,
		maccPerms,
		accountAddressPrefix,
		govAuthority,
	)
	bankKeeper := bankkeeper.NewBaseKeeper(
		appCodec,
		keys[banktypes.StoreKey],
		accountKeeper,
		make(map[string]bool),
		govAuthority,
	)
	stakingKeeper := stakingkeeper.NewKeeper(
		appCodec,
		keys[stakingtypes.StoreKey],
		accountKeeper,
		bankKeeper,
		govAuthority,
	)
	mintKeeper := mintkeeper.NewKeeper(
		appCodec,
		keys[minttypes.StoreKey],
		stakingKeeper,
		accountKeeper,
		bankKeeper,
		authtypes.FeeCollectorName,
		govAuthority,
	)
	rollupKeeper := rollupkeeper.NewKeeper(
		appCodec,
		keys[rolluptypes.StoreKey],
		keys[rolluptypes.MemStoreKey],
		&mintKeeper,
		bankKeeper,
	)

	mm := module.NewManager(
		auth.NewAppModule(appCodec, accountKeeper, authsims.RandomGenesisAccounts, paramstypes.Subspace{}),
		bank.NewAppModule(appCodec, bankKeeper, accountKeeper, paramstypes.Subspace{}),
		staking.NewAppModule(appCodec, stakingKeeper, accountKeeper, bankKeeper, paramstypes.Subspace{}),
		mint.NewAppModule(appCodec, mintKeeper, accountKeeper, minttypes.DefaultInflationCalculationFn, paramstypes.Subspace{}),
		rollup.NewAppModule(appCodec, rollupKeeper),
		testmodule.New(keys[testmodule.StoreKey]),
	)

	mm.SetOrderBeginBlockers(
		minttypes.ModuleName,
		stakingtypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		govtypes.ModuleName,
		rolluptypes.ModuleName,
		testmodule.ModuleName,
	)
	mm.SetOrderEndBlockers(
		govtypes.ModuleName,
		stakingtypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		minttypes.ModuleName,
		rolluptypes.ModuleName,
		testmodule.ModuleName,
	)
	genesisModuleOrder := []string{
		authtypes.ModuleName,
		banktypes.ModuleName,
		stakingtypes.ModuleName,
		govtypes.ModuleName,
		minttypes.ModuleName,
		paramstypes.ModuleName,
		rolluptypes.ModuleName,
		testmodule.ModuleName,
	}
	mm.SetOrderInitGenesis(genesisModuleOrder...)
	mm.SetOrderExportGenesis(genesisModuleOrder...)
	mm.RegisterServices(module.NewConfigurator(appCodec, ba.MsgServiceRouter(), ba.GRPCQueryRouter()))

	ba.SetInitChainer(func(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
		var genesisState map[string]json.RawMessage
		if err := tmjson.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
			panic(fmt.Errorf("unmarshal genesis state: %v", err))
		}
		return mm.InitGenesis(ctx, appCodec, genesisState)
	})
	ba.SetBeginBlocker(func(ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
		return mm.BeginBlock(ctx, req)
	})
	ba.SetEndBlocker(func(ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
		return mm.EndBlock(ctx, req)
	})
	ba.MountKVStores(keys)

	// If we don't LoadLatestVersion, the module store won't be loaded.
	// I don't understand the full meaning or implications of this.
	if err := ba.LoadLatestVersion(); err != nil {
		panic(fmt.Errorf("load latest version: %v", err))
	}

	return &App{
		BaseApp:      ba,
		basicManager: basicManager,
		codec:        appCodec,
	}
}

func (a *App) DefaultGenesis() map[string]json.RawMessage {
	return a.basicManager.DefaultGenesis(a.codec)
}
