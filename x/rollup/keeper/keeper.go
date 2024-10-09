package keeper

import (
	"context"
	"fmt"
	"math"

	"cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type Keeper struct {
	cdc           codec.BinaryCodec
	storeService  store.KVStoreService
	rollupCfg     *rollup.Config
	bankkeeper    types.BankKeeper
	accountKeeper types.AccountKeeper
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
		accountKeeper: accountKeeper,
		rollupCfg:     &rollup.Config{},
	}
}

func (k *Keeper) InitGenesis(ctx context.Context) error {
	baseAccount, err := authtypes.NewBaseAccountWithPubKey(monomer.PrivKey.PubKey())
	if err != nil {
		return fmt.Errorf("new base account with pub key: %v", err)
	}
	k.accountKeeper.NewAccount(ctx, baseAccount)
	k.accountKeeper.SetAccount(ctx, baseAccount)

	coin := sdk.NewCoin(types.ETH, sdkmath.NewInt(math.MaxInt)) // max out so the dummy signer doesn't run out of gas
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to mint ETH deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, baseAccount.GetAddress(), sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to send ETH deposit coins from rollup module to user account %v: %v", baseAccount.GetAddress(), err)
	}
	return nil
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
