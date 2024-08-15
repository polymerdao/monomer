package types_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

func TestMsgInitiateWithdrawalValidateBasic(t *testing.T) {
	validAddress := "0x311d373126EFAE95E261DefF004FF245021739d1"

	invalidAddress := "invalid address"
	invalidAddressErrorMsg := "invalid Ethereum address"

	validGasLimit := new(big.Int).SetUint64(types.MinTxGasLimit/2 + types.MaxTxGasLimit/2).Bytes() // avoid overflow
	belowRangeGasLimit := new(big.Int).SetUint64(types.MinTxGasLimit - 1).Bytes()
	aboveRangeGasLimit := new(big.Int).SetUint64(types.MaxTxGasLimit + 1).Bytes()

	outOfRangeGasLimitErrorMsg := fmt.Sprintf("gas limit must be between %d and %d:", types.MinTxGasLimit, types.MaxTxGasLimit)

	testCases := []struct {
		name    string
		request *types.MsgInitiateWithdrawal
		errMsg  string
	}{
		{
			name: "Valid request",
			request: &types.MsgInitiateWithdrawal{
				Target:   validAddress,
				GasLimit: validGasLimit,
			},
		},
		{
			name: "Invalid Ethereum address",
			request: &types.MsgInitiateWithdrawal{
				Target:   invalidAddress,
				GasLimit: validGasLimit,
			},
			errMsg: invalidAddressErrorMsg,
		},
		{
			name: "Gas limit below the allowed range",
			request: &types.MsgInitiateWithdrawal{
				Target:   validAddress,
				GasLimit: belowRangeGasLimit,
			},
			errMsg: outOfRangeGasLimitErrorMsg,
		},
		{
			name: "Gas limit above the allowed range",
			request: &types.MsgInitiateWithdrawal{
				Target:   validAddress,
				GasLimit: aboveRangeGasLimit,
			},
			errMsg: outOfRangeGasLimitErrorMsg,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.ValidateBasic()
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
		})
	}
}
