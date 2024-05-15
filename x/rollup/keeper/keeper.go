package keeper

import (
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	rollupCfg    *rollup.Config
	mintKeeper   *mintkeeper.Keeper
	bankkeeper   bankkeeper.Keeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	// dependencies
	mintKeeper *mintkeeper.Keeper,
	bankKeeper bankkeeper.Keeper,
) *Keeper {
	return &Keeper{
		cdc:          cdc,
		storeService: storeService,
		mintKeeper:   mintKeeper,
		bankkeeper:   bankKeeper,
		rollupCfg:    &rollup.Config{},
	}
}
