package peptide

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

// GetETH returns the ETH balance of an account in wei
func (a *PeptideApp) GetETH(addr sdk.AccAddress, height int64) (*sdkmath.Int, error) {
	resp := MustGetResponseWithHeight(new(banktypes.QueryBalanceResponse), a, &banktypes.QueryBalanceRequest{Address: addr.String(), Denom: rolluptypes.ETH}, "/cosmos.bank.v1beta1.Query/Balance", height)
	return &resp.Balance.Amount, nil
}
