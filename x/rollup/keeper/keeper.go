package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
)

type Keeper struct {
	cdc        codec.BinaryCodec
	storeKey   storetypes.StoreKey
	memKey     storetypes.StoreKey
	rollupCfg  *rollup.Config
	mintKeeper *mintkeeper.Keeper
	bankkeeper bankkeeper.Keeper
}

// NewKeeper create a new polyibc core keeper instance
func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey,
	memKey storetypes.StoreKey,
	// dependencies
	mintKeeper *mintkeeper.Keeper,
	bankkeeper bankkeeper.Keeper,

) *Keeper {
	return &Keeper{
		cdc:        cdc,
		storeKey:   storeKey,
		memKey:     memKey,
		mintKeeper: mintKeeper,
		bankkeeper: bankkeeper,
		rollupCfg:  &rollup.Config{},
	}
}
