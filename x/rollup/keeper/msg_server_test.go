package keeper_test

import (
	"fmt"
	"math/big"
	"testing"

	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	"github.com/polymerdao/monomer/x/rollup/keeper"
	"github.com/stretchr/testify/require"
)

func TestValidateBasic(t *testing.T) {
	validAddress := "0x311d373126EFAE95E261DefF004FF245021739d1"

	invalidAddress := "invalid address"
	invalidAddressErrorMsg := "invalid Ethereum address"

	validGasLimit := new(big.Int).SetUint64(keeper.MinTxGasLimit/2 + keeper.MaxTxGasLimit/2).Bytes() // avoid overflow
	belowRangeGasLimit := new(big.Int).SetUint64(keeper.MinTxGasLimit - 1).Bytes()
	aboveRangeGasLimit := new(big.Int).SetUint64(keeper.MaxTxGasLimit + 1).Bytes()

	outOfRangeGasLimitErrorMsg := fmt.Sprintf("gas limit must be between %d and %d:", keeper.MinTxGasLimit, keeper.MaxTxGasLimit)

	testCases := []struct {
		name    string
		request *rollupv1.InitiateWithdrawalRequest
		errMsg  string
	}{
		{
			name: "Valid request",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   validAddress,
				GasLimit: validGasLimit,
			},
		},
		{
			name: "Invalid Ethereum address",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   invalidAddress,
				GasLimit: validGasLimit,
			},
			errMsg: invalidAddressErrorMsg,
		},
		{
			name: "Gas limit below the allowed range",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   validAddress,
				GasLimit: belowRangeGasLimit,
			},
			errMsg: outOfRangeGasLimitErrorMsg,
		},
		{
			name: "Gas limit above the allowed range",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   validAddress,
				GasLimit: aboveRangeGasLimit,
			},
			errMsg: outOfRangeGasLimitErrorMsg,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := keeper.ValidateBasic(tc.request)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
		})
	}
}
