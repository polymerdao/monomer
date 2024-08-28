package utils

import (
	"fmt"
	"io"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hashicorp/go-multierror"
)

func Ptr[T any](x T) *T {
	return &x
}

func WrapCloseErr(err error, closer io.Closer) error {
	closeErr := closer.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close: %v", closeErr)
	}
	if err != nil || closeErr != nil {
		return multierror.Append(err, closeErr)
	}
	return nil
}

// EvmToCosmosAddress converts an EVM address to a sdktypes.AccAddress
func EvmToCosmosAddress(addr common.Address) sdktypes.AccAddress {
	return addr.Bytes()
}
