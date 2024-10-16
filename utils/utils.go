package utils

import (
	"fmt"
	"io"

	"github.com/cosmos/cosmos-sdk/types/bech32"
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
func EvmToCosmosAddress(prefix string, ethAddr common.Address) string {
	addr, err := bech32.ConvertAndEncode(prefix, ethAddr.Bytes())
	if err != nil {
		panic(fmt.Errorf("convert and encode: %v", err))
	}
	return addr
}
