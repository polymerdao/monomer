package integrations

import (
	"fmt"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	rolluptx "github.com/polymerdao/monomer/x/rollup/tx"
)

type AnteHandler struct {
	authAnte sdktypes.AnteHandler
}

func NewAnteHandler(options authante.HandlerOptions) (*AnteHandler, error) {
	authAnteHandler, err := authante.NewAnteHandler(options)
	if err != nil {
		return nil, fmt.Errorf("new auth ante handler: %v", err)
	}
	return &AnteHandler{
		authAnte: authAnteHandler,
	}, nil
}

func (a *AnteHandler) AnteHandle(ctx sdktypes.Context, tx sdktypes.Tx, simulate bool) (sdktypes.Context, error) {
	switch tx.(type) {
	case *rolluptx.Deposit:
		newCtx, err := rolluptx.DepositAnteHandler(ctx, tx, simulate)
		if err != nil {
			return newCtx, fmt.Errorf("deposit ante handle: %v", err)
		}
		return newCtx, err
	default: // Unfortunately, the Cosmos SDK does not export its default tx type.
		newCtx, err := a.authAnte(ctx, tx, simulate)
		if err != nil {
			return newCtx, fmt.Errorf("auth ante handle: %v", err)
		}
		return newCtx, nil
	}
}
