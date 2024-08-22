package testapp

import (
	"context"
	"encoding/json"
	"fmt"

	runtimev1alpha1 "cosmossdk.io/api/cosmos/app/runtime/v1alpha1"
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	bankmodulev1 "cosmossdk.io/api/cosmos/bank/module/v1"
	stakingmodulev1 "cosmossdk.io/api/cosmos/staking/module/v1"
	txconfigv1 "cosmossdk.io/api/cosmos/tx/config/v1"
	"cosmossdk.io/core/appconfig"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	_ "github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	_ "github.com/cosmos/cosmos-sdk/x/auth/tx"
	_ "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	_ "github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	_ "github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	rollupmodulev1 "github.com/polymerdao/monomer/gen/rollup/module/v1"
	testappmodulev1 "github.com/polymerdao/monomer/gen/testapp/module/v1"
	"github.com/polymerdao/monomer/testapp/x/testmodule"
	testmodulekeeper "github.com/polymerdao/monomer/testapp/x/testmodule/keeper"
	_ "github.com/polymerdao/monomer/x/rollup"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

// App is an app with the absolute minimum amount of configuration required to have the Monomer rollup module.
// It also has a dummy test module for easy transaction testing.
// The test module will initialize a single validator to satisfy the module manager's InitChain invariant that the validator set must be non-empty
// (the requirement doesn't make sense to me since that's a consensus-layer concern).
type App struct {
	app            *runtime.App
	defaultGenesis map[string]json.RawMessage
}

func (a *App) Info(_ context.Context, r *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error) {
	return a.app.Info(r)
}

func (a *App) Query(ctx context.Context, r *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error) {
	return a.app.Query(ctx, r)
}

func (a *App) CheckTx(_ context.Context, r *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error) {
	return a.app.CheckTx(r)
}

func (a *App) InitChain(_ context.Context, r *abcitypes.RequestInitChain) (*abcitypes.ResponseInitChain, error) {
	return a.app.InitChain(r)
}

func (a *App) FinalizeBlock(_ context.Context, r *abcitypes.RequestFinalizeBlock) (*abcitypes.ResponseFinalizeBlock, error) {
	return a.app.FinalizeBlock(r)
}

func (a *App) Commit(_ context.Context, _ *abcitypes.RequestCommit) (*abcitypes.ResponseCommit, error) {
	return a.app.Commit()
}

func (a *App) RollbackToHeight(_ context.Context, targetHeight uint64) error {
	return a.app.CommitMultiStore().RollbackToVersion(int64(targetHeight))
}

var modules = []string{
	authtypes.ModuleName,
	banktypes.ModuleName,
	stakingtypes.ModuleName,
	govtypes.ModuleName,
	testmodule.ModuleName,
	rolluptypes.ModuleName,
}

func New(appdb dbm.DB, chainID string) (*App, error) {
	config := &appv1alpha1.Config{
		Modules: []*appv1alpha1.ModuleConfig{
			{
				Name: runtime.ModuleName,
				Config: appconfig.WrapAny(&runtimev1alpha1.Module{
					AppName:       "TestApp",
					PreBlockers:   modules,
					BeginBlockers: modules,
					EndBlockers:   modules,
					InitGenesis:   modules,
				}),
			},
			{
				Name: authtypes.ModuleName,
				Config: appconfig.WrapAny(&authmodulev1.Module{
					Bech32Prefix: "cosmos",
					ModuleAccountPermissions: []*authmodulev1.ModuleAccountPermission{
						{
							Account: authtypes.FeeCollectorName,
						},
						{
							Account:     rolluptypes.ModuleName,
							Permissions: []string{authtypes.Minter, authtypes.Burner},
						},
						{
							Account:     stakingtypes.BondedPoolName,
							Permissions: []string{authtypes.Burner, stakingtypes.ModuleName},
						},
						{
							Account:     stakingtypes.NotBondedPoolName,
							Permissions: []string{authtypes.Burner, stakingtypes.ModuleName},
						},
					},
				}),
			},
			{
				Name:   banktypes.ModuleName,
				Config: appconfig.WrapAny(&bankmodulev1.Module{}),
			},
			{
				Name: stakingtypes.ModuleName,
				Config: appconfig.WrapAny(&stakingmodulev1.Module{
					Bech32PrefixValidator: "cosmosvaloper",
					Bech32PrefixConsensus: "cosmosvalcons",
				}),
			},
			{
				Name: "tx",
				Config: appconfig.WrapAny(&txconfigv1.Config{
					SkipAnteHandler: true, // Ignore signatures and gas for testing.
				}),
			},
			{
				Name:   testmodule.ModuleName,
				Config: appconfig.WrapAny(&testappmodulev1.Module{}),
			},
			{
				Name:   rolluptypes.ModuleName,
				Config: appconfig.WrapAny(&rollupmodulev1.Module{}),
			},
		},
	}
	var (
		appBuilder        *runtime.AppBuilder
		appCodec          codec.Codec
		legacyAmino       *codec.LegacyAmino
		txConfig          client.TxConfig
		interfaceRegistry codectypes.InterfaceRegistry

		accountKeeper    authkeeper.AccountKeeper
		bankKeeper       bankkeeper.Keeper
		stakingKeeper    *stakingkeeper.Keeper
		rollupKeeper     *rollupkeeper.Keeper
		testmoduleKeeper *testmodulekeeper.Keeper
	)
	if err := depinject.Inject(depinject.Configs(appconfig.Compose(config), depinject.Supply(log.NewNopLogger())),
		&appBuilder,
		&appCodec,
		&legacyAmino,
		&txConfig,
		&interfaceRegistry,
		&accountKeeper,
		&bankKeeper,
		&stakingKeeper,
		&rollupKeeper,
		&testmoduleKeeper,
	); err != nil {
		return nil, fmt.Errorf("inject config: %v", err)
	}

	runtimeApp := appBuilder.Build(appdb, nil, baseapp.SetChainID(chainID))

	runtimeApp.SetInitChainer(func(ctx sdktypes.Context, req *abcitypes.RequestInitChain) (*abcitypes.ResponseInitChain, error) {
		var genesisState map[string]json.RawMessage
		if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
			return nil, fmt.Errorf("unmarshal genesis state: %v", err)
		}
		return runtimeApp.ModuleManager.InitGenesis(ctx, appCodec, genesisState)
	})

	if err := runtimeApp.LoadLatestVersion(); err != nil {
		return nil, fmt.Errorf("load latest version: %v", err)
	}

	return &App{
		app:            runtimeApp,
		defaultGenesis: appBuilder.DefaultGenesis(),
	}, nil
}

// DefaultGenesis returns the app's default genesis state. It must be cloned before it is modified.
func (a *App) DefaultGenesis() map[string]json.RawMessage {
	return a.defaultGenesis
}
