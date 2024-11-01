package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type Keeper struct {
	cdc           codec.BinaryCodec
	storeService  store.KVStoreService
	rollupCfg     *rollup.Config
	bankkeeper    types.BankKeeper
	accountkeeper types.AccountKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	// dependencies
	bankKeeper types.BankKeeper,
	accountKeeper types.AccountKeeper,
) *Keeper {
	return &Keeper{
		cdc:           cdc,
		storeService:  storeService,
		bankkeeper:    bankKeeper,
		accountkeeper: accountKeeper,
		rollupCfg:     &rollup.Config{},
	}
}

// EmitEvents prepares a `message` event with the module name and emits it
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

func (k *Keeper) GetL1BlockInfo(ctx sdk.Context) (*types.L1BlockInfo, error) { //nolint:gocritic // hugeParam
	l1BlockInfoBz, err := k.storeService.OpenKVStore(ctx).Get([]byte(types.KeyL1BlockInfo))
	if err != nil {
		return nil, fmt.Errorf("get l1 block info: %w", err)
	}
	var l1BlockInfo types.L1BlockInfo
	if err = l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return nil, fmt.Errorf("unmarshal l1 block info: %w", err)
	}
	return &l1BlockInfo, nil
}
