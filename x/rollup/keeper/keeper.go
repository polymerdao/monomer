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
	authority     sdk.AccAddress
	rollupCfg     *rollup.Config
	bankkeeper    types.BankKeeper
	accountkeeper types.AccountKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority sdk.AccAddress,
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
		authority:     authority,
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
	l1BlockInfoBz, err := k.storeService.OpenKVStore(ctx).Get([]byte(types.L1BlockInfoKey))
	if err != nil {
		return nil, fmt.Errorf("get l1 block info: %w", err)
	}
	var l1BlockInfo types.L1BlockInfo
	if err = l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return nil, fmt.Errorf("unmarshal l1 block info: %w", err)
	}
	return &l1BlockInfo, nil
}

// SetL1BlockInfo sets the derived L1 block info in the rollup store.
//
// Persisted data conforms to optimism specs on L1 attributes:
// https://github.com/ethereum-optimism/optimism/blob/develop/specs/deposits.md#l1-attributes-predeployed-contract
func (k *Keeper) SetL1BlockInfo(ctx sdk.Context, info types.L1BlockInfo) error { //nolint:gocritic
	infoBytes, err := info.Marshal()
	if err != nil {
		return types.WrapError(err, "marshal L1 block info")
	}
	if err = k.storeService.OpenKVStore(ctx).Set([]byte(types.L1BlockInfoKey), infoBytes); err != nil {
		return types.WrapError(err, "set latest L1 block info")
	}
	return nil
}

func (k *Keeper) GetParams(ctx sdk.Context) (*types.Params, error) { //nolint:gocritic // hugeParam
	paramsBz, err := k.storeService.OpenKVStore(ctx).Get([]byte(types.ParamsKey))
	if err != nil {
		return nil, fmt.Errorf("get params: %w", err)
	}
	var params types.Params
	if err = params.Unmarshal(paramsBz); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}
	return &params, nil
}

func (k *Keeper) SetParams(ctx sdk.Context, params *types.Params) error { //nolint:gocritic // hugeParam
	paramsBz, err := params.Marshal()
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	err = k.storeService.OpenKVStore(ctx).Set([]byte(types.ParamsKey), paramsBz)
	if err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	return nil
}

// getERC20Denom returns the Monomer L2 coin denom for the given ERC-20 L1 address.
// The "erc20/{l1erc20addr}" format is used for the L2 coin denom.
func getERC20Denom(erc20Addr string) string {
	return "erc20/" + erc20Addr[2:]
}
