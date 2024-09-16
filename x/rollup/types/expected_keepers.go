package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

//go:generate mockgen -destination ../testutil/expected_keepers_mocks.go -package=testutil . BankKeeper

// BankKeeper defines the expected bank keeper interface used in the x/rollup module
type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error

	MintCoins(ctx context.Context, name string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}
