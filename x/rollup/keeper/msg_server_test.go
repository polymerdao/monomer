package keeper_test

import (
	"encoding/binary"
	"testing"

	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	"github.com/polymerdao/monomer/x/rollup/keeper"
	"github.com/stretchr/testify/require"
)

func TestValidateBasic(t *testing.T) {
	validAddress := "0x311d373126EFAE95E261DefF004FF245021739d1"
	invalidAddress := "invalid address"

	validGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(validGasLimit, 100_000)

	belowRangeGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(belowRangeGasLimit, 4_999)

	aboveRangeGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(aboveRangeGasLimit, 9_223_372_036_854_775_808)

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
			errMsg: "",
		},
		{
			name: "Invalid Ethereum address",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   invalidAddress,
				GasLimit: validGasLimit,
			},
			errMsg: "invalid Ethereum address",
		},
		{
			name: "Gas limit below the allowed range",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   validAddress,
				GasLimit: belowRangeGasLimit,
			},
			errMsg: "gas limit must be between 5,000 and 9,223,372,036,854,775,807",
		},
		{
			name: "Gas limit above the allowed range",
			request: &rollupv1.InitiateWithdrawalRequest{
				Target:   validAddress,
				GasLimit: aboveRangeGasLimit,
			},
			errMsg: "gas limit must be between 5,000 and 9,223,372,036,854,775,807",
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
