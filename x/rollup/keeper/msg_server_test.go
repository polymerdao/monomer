package keeper

import (
	"encoding/binary"
	"testing"

	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	"github.com/stretchr/testify/require"
)

func TestValidateBasic(t *testing.T) {
	validAddress := "0x311d373126EFAE95E261DefF004FF245021739d1"
	invalidAddress := "invalid address"

	validGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(validGasLimit, 100_000)

	belowRangeGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(belowRangeGasLimit, 4999)

	aboveRangeGasLimit := make([]byte, 8)
	binary.BigEndian.PutUint64(belowRangeGasLimit, 9_223_372_036_854_775_808)

	// Valid Ethereum address and gas limit within the allowed range
	validRequest := &rollupv1.InitiateWithdrawalRequest{
		Target:   validAddress,
		GasLimit: validGasLimit,
	}

	err := validateBasic(validRequest)
	require.NoError(t, err)

	// Invalid Ethereum address
	invalidAddressRequest := &rollupv1.InitiateWithdrawalRequest{
		Target:   invalidAddress,
		GasLimit: validGasLimit,
	}

	err = validateBasic(invalidAddressRequest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid Ethereum address")

	// Gas limit below the allowed range
	lowGasLimitRequest := &rollupv1.InitiateWithdrawalRequest{
		Target:   validAddress,
		GasLimit: belowRangeGasLimit,
	}

	err = validateBasic(lowGasLimitRequest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gas limit must be between 5,000 and 9,223,372,036,854,775,807")

	// Gas limit above the allowed range
	highGasLimitRequest := &rollupv1.InitiateWithdrawalRequest{
		Target:   validAddress,
		GasLimit: aboveRangeGasLimit,
	}
	err = validateBasic(highGasLimitRequest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gas limit must be between 5,000 and 9,223,372,036,854,775,807")
}
