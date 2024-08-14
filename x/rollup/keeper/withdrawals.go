package keeper

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
)

// burnETH burns ETH from an account where the amount is in wei.
func (k *Keeper) burnETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) error { //nolint:gocritic // hugeParam
	coins := sdk.NewCoins(sdk.NewCoin(types.ETH, amount))

	// Transfer the coins to withdraw from the user account to the mint module
	if err := k.bankkeeper.SendCoinsFromAccountToModule(ctx, addr, types.MintModule, coins); err != nil {
		return fmt.Errorf("failed to send withdrawal coins from user account %v to mint module: %v", addr, err)
	}

	// Burn the ETH coins from the mint module
	if err := k.bankkeeper.BurnCoins(ctx, types.MintModule, coins); err != nil {
		return fmt.Errorf("failed to burn withdrawal coins from mint module: %v", err)
	}

	return nil
}
