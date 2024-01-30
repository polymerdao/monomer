package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type Keeper struct {
	cdc        codec.BinaryCodec
	storeKey   storetypes.StoreKey
	memKey     storetypes.StoreKey
	paramStore paramtypes.Subspace
	rollupCfg  *rollup.Config
	mintKeeper *mintkeeper.Keeper
	bankkeeper bankkeeper.Keeper
}

type RollUpKeeperReadOnly interface {
	GetL1BlockHistory(rollupCfg *rollup.Config, ctx sdk.Context, blockHash []byte) (derive.L1BlockInfo, error)
}

// NewKeeper create a new polyibc core keeper instance
func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey,
	memKey storetypes.StoreKey,
	ps paramtypes.Subspace,
	// dependencies
	mintKeeper *mintkeeper.Keeper,
	bankkeeper bankkeeper.Keeper,

) *Keeper {
	// set KeyTable if it has not already been set
	if !ps.HasKeyTable() {
		ps = ps.WithKeyTable(types.ParamKeyTable())
	}

	return &Keeper{
		cdc:        cdc,
		storeKey:   storeKey,
		memKey:     memKey,
		paramStore: ps,
		mintKeeper: mintKeeper,
		bankkeeper: bankkeeper,
		rollupCfg:  &rollup.Config{},
	}
}
