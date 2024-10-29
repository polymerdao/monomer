package helpers

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	txsigning "cosmossdk.io/x/tx/signing"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptx "github.com/polymerdao/monomer/x/rollup/tx"
)

type AnteHandler struct {
	authAnteHandler   sdktypes.AnteHandler
	l1DataAnteHandler sdktypes.AnteHandler
}

// HandlerOptions are the options required for constructing the AnteHandler used for non-deposit txs.
// The options are the same as those required for constructing the default x/auth AnteHandler with the
// addition of the RollupKeeper.
type HandlerOptions struct {
	AccountKeeper          authante.AccountKeeper
	BankKeeper             authtypes.BankKeeper
	RollupKeeper           *rollupkeeper.Keeper
	ExtensionOptionChecker authante.ExtensionOptionChecker
	FeegrantKeeper         authante.FeegrantKeeper
	SignModeHandler        *txsigning.HandlerMap
	SigGasConsumer         func(meter storetypes.GasMeter, sig signing.SignatureV2, params authtypes.Params) error
	TxFeeChecker           authante.TxFeeChecker
}

func NewAnteHandler(options HandlerOptions) (*AnteHandler, error) { //nolint:gocritic // hugeParam
	authAnteHandler, err := authante.NewAnteHandler(authante.HandlerOptions{
		AccountKeeper:          options.AccountKeeper,
		BankKeeper:             options.BankKeeper,
		ExtensionOptionChecker: options.ExtensionOptionChecker,
		FeegrantKeeper:         options.FeegrantKeeper,
		SignModeHandler:        options.SignModeHandler,
		SigGasConsumer:         options.SigGasConsumer,
		TxFeeChecker:           options.TxFeeChecker,
	})
	if err != nil {
		return nil, fmt.Errorf("new auth ante handler: %v", err)
	}

	l1DataAnteHandler, err := NewL1DataAnteHandler(options)
	if err != nil {
		return nil, fmt.Errorf("new l1 data ante handler: %v", err)
	}

	return &AnteHandler{
		authAnteHandler:   authAnteHandler,
		l1DataAnteHandler: l1DataAnteHandler,
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
		newCtx, err = a.l1DataAnteHandler(newCtx, tx, simulate)
		if err != nil {
			return newCtx, fmt.Errorf("l1 data ante handle: %v", err)
		}
		return newCtx, nil
	}
}
