package helpers

import (
	"fmt"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptx "github.com/polymerdao/monomer/x/rollup/tx"
)

type AnteHandler struct {
	authAnteHandler sdktypes.AnteHandler
	rollupKeeper    *rollupkeeper.Keeper
}

func NewAnteHandler(
	options authante.HandlerOptions, //nolint:gocritic // hugeParam
	rollupKeeper *rollupkeeper.Keeper,
) (*AnteHandler, error) {
	authAnteHandler, err := authante.NewAnteHandler(options)
	if err != nil {
		return nil, fmt.Errorf("new auth ante handler: %v", err)
	}

	return &AnteHandler{
		authAnteHandler: authAnteHandler,
		rollupKeeper:    rollupKeeper,
	}, nil
}

func (a *AnteHandler) AnteHandle(
	ctx sdktypes.Context, //nolint:gocritic // hugeParam
	tx sdktypes.Tx,
	simulate bool,
) (sdktypes.Context, error) {
	switch tx.(type) {
	case *rolluptx.Deposit:
		newCtx, err := rolluptx.DepositAnteHandler(ctx, tx, simulate)
		if err != nil {
			return newCtx, fmt.Errorf("deposit ante handle: %v", err)
		}
		return newCtx, err
	default: // Unfortunately, the Cosmos SDK does not export its default tx type.
		newCtx, err := a.authAnteHandler(ctx, tx, simulate)
		if err != nil {
			return newCtx, fmt.Errorf("auth ante handle: %v", err)
		}
		newCtx, err = rolluptx.L1DataAnteHandler(newCtx, tx, a.rollupKeeper)
		if err != nil {
			return newCtx, fmt.Errorf("l1 data ante handle: %v", err)
		}
		return newCtx, nil
	}
}
