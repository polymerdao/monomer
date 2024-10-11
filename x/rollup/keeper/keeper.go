package keeper

import (
	"context"

	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	rollupCfg    *rollup.Config
	bankkeeper   types.BankKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	// dependencies
	bankKeeper types.BankKeeper,
) *Keeper {
	return &Keeper{
		cdc:          cdc,
		storeService: storeService,
		bankkeeper:   bankKeeper,
		rollupCfg:    &rollup.Config{},
	}
}

// Helper. Prepares a `message` event with the module name and emits it
// along with the provided events.
func (k *Keeper) EmitEvents(goCtx context.Context, events sdk.Events) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	moduleEvent := sdk.NewEvent(
		sdk.EventTypeMessage,
		sdk.NewAttribute(sdk.AttributeKeyModule, types.ModuleName),
	)
	events = append(sdk.Events{moduleEvent}, events...)

	ctx.EventManager().EmitEvents(events)
}
