package integrations

import (
	"errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
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
	for _, decoder := range []sdktypes.TxDecoder{
		d.authTxDecoder,
		rolluptx.DepositDecoder,
	} {
		if tx, err := decoder(txBytes); err == nil {
			return tx, nil
		}
	}
	return nil, errors.New("failed to decode tx")
}
