package helpers

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/hashicorp/go-multierror"
	rolluptx "github.com/polymerdao/monomer/x/rollup/tx"
)

type TxDecoder struct {
	authTxDecoder sdktypes.TxDecoder
}

func NewTxDecoder(cdc codec.Codec) *TxDecoder {
	return &TxDecoder{
		authTxDecoder: authtx.DefaultTxDecoder(cdc),
	}
}

func (d *TxDecoder) Decode(txBytes []byte) (sdktypes.Tx, error) {
	var err error
	for name, decoder := range map[string]sdktypes.TxDecoder{
		"auth tx":    d.authTxDecoder,
		"deposit tx": rolluptx.DepositDecoder,
	} {
		tx, decodeErr := decoder(txBytes)
		if decodeErr == nil {
			return tx, nil
		}
		err = multierror.Append(err, fmt.Errorf("%s: %v", name, decodeErr))
	}
	return nil, err
}
