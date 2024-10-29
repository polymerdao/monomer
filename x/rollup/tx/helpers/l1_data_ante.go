package helpers

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// NewL1DataAnteHandler has a single decorator (NewConsumeGasForL1DataDecorator)
// to consume gas to compensate the sequencer for posting the transaction to Ethereum.
func NewL1DataAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) { //nolint:gocritic // hugeparam
	if options.RollupKeeper == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "rollup keeper is required for ante builder")
	}

	anteDecorators := []sdk.AnteDecorator{
		NewConsumeGasForL1DataDecorator(options.RollupKeeper),
	}

	return sdk.ChainAnteDecorators(anteDecorators...), nil
}
