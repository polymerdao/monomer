package tx

import (
	"fmt"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/tx/internal"
	"github.com/polymerdao/monomer/x/rollup/types"
)

func DepositAnteHandler(ctx sdktypes.Context, tx sdktypes.Tx, simulate bool) (sdktypes.Context, error) { //nolint:gocritic // hugeparam
	if _, ok := tx.(*types.DepositsTx); ok {
		ctx = ctx.WithGasMeter(internal.NewFreeInfiniteGasMeter())
	}
	return ctx, nil
}

// DepositDecoder is an sdktypes.TxDecoder.
func DepositDecoder(txBytes []byte) (sdktypes.Tx, error) {
	depositsTx := new(types.DepositsTx)
	if err := depositsTx.Unmarshal(txBytes); err != nil {
		return nil, fmt.Errorf("unmarshal deposits tx: %v", err)
	}
	return depositsTx, nil
}
