package keeper

import (
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	rollupCfg    *rollup.Config
	bankkeeper   bankkeeper.Keeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	// dependencies
	bankKeeper bankkeeper.Keeper,
) *Keeper {
	return &Keeper{
		cdc:          cdc,
		storeService: storeService,
		bankkeeper:   bankKeeper,
		rollupCfg:    &rollup.Config{},
	}
}
