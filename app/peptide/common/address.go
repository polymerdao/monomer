package common

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

// CosmosToEvm converts a sdk.AccAddress to an EVM address
func CosmosToEvm(addr sdk.AccAddress) ethcommon.Address {
	return ethcommon.BytesToAddress(addr.Bytes())
}

// EvmToCosmos converts an EVM address to a sdk.AccAddress
func EvmToCosmos(addr ethcommon.Address) sdk.AccAddress {
	return sdk.AccAddress(addr.Bytes())
}
