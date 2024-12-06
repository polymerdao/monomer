package keeper

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
)

// burnETH burns ETH from an account where the amount is in wei.
func (k *Keeper) burnETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) error { //nolint:gocritic // hugeParam
	coins := sdk.NewCoins(sdk.NewCoin(types.WEI, amount))

	// Transfer the coins to withdraw from the user account to the rollup module
	if err := k.bankkeeper.SendCoinsFromAccountToModule(ctx, addr, types.ModuleName, coins); err != nil {
		return fmt.Errorf("failed to send withdrawal coins from user account %v to rollup module: %v", addr, err)
	}

	// Burn the ETH coins from the rollup module
	if err := k.bankkeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return fmt.Errorf("failed to burn withdrawal coins from rollup module: %v", err)
	}

	return nil
}

// burnERC20 burns a bridged ERC-20 token from an L2 account.
func (k *Keeper) burnERC20(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	userAddr sdk.AccAddress,
	erc20addr string,
	amount sdkmath.Int,
) error {
	coins := sdk.NewCoins(sdk.NewCoin(getERC20Denom(erc20addr), amount))

	// Transfer the coins to withdraw from the user account to the rollup module
	if err := k.bankkeeper.SendCoinsFromAccountToModule(ctx, userAddr, types.ModuleName, coins); err != nil {
		return fmt.Errorf("failed to send withdrawal coins from user account %v to rollup module: %v", userAddr, err)
	}

	if err := k.bankkeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return fmt.Errorf("failed to burn ERC-20 coins from the rollup module: %v", err)
	}

	return nil
}
