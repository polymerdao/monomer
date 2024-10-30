package tx

import (
	"fmt"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/rlp"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
)

// L1DataAnteHandler will consume gas to compensate the sequencer for posting
// the transaction to Ethereum. The gas cost is calculated based on the Ecotone
// upgrade and the sequencer is expected to post the transaction using blobs.
func L1DataAnteHandler(ctx sdk.Context, tx sdk.Tx, rollupKeeper *rollupkeeper.Keeper) (sdk.Context, error) { //nolint:gocritic // hugeparam
	if rollupKeeper == nil {
		return ctx, errorsmod.Wrap(sdkerrors.ErrLogic, "rollup keeper is required for l1 data ante handler")
	}

	l1BlockInfo, err := rollupKeeper.GetL1BlockInfo(ctx)
	if err != nil {
		return ctx, fmt.Errorf("get l1 block info: %w", err)
	}

	baseFee := new(big.Int).SetBytes(l1BlockInfo.BaseFee)
	baseFeeScalar := big.NewInt(int64(l1BlockInfo.BaseFeeScalar))
	blobBaseFee := new(big.Int).SetBytes(l1BlockInfo.BlobBaseFee)
	blobBaseFeeScalar := big.NewInt(int64(l1BlockInfo.BlobBaseFeeScalar))

	l1GasUsed, err := getL1GasUsed(tx)
	if err != nil {
		return ctx, fmt.Errorf("get l1 gas used: %w", err)
	}

	const baseFeeMultiplier = 16
	const divisor = 16000000 // 16e6

	// scaledBaseFee calculation: scaledBaseFee = l1BaseFee * lBaseFeeScalar * 16
	scaledBaseFee := new(big.Int).Mul(new(big.Int).Mul(baseFee, baseFeeScalar), big.NewInt(baseFeeMultiplier))

	// scaledBlobBaseFee calculation: scaledBlobBaseFee = l1BlobBaseFee * l1BlobBaseFeeScalar
	scaledBlobBaseFee := new(big.Int).Mul(blobBaseFee, blobBaseFeeScalar)

	// l1DataCost calculation: l1DataCost = [l1GasUsed * (scaledBaseFee + scaledBlobBaseFee)] / 16e6
	l1DataCost := new(big.Int).Div(new(big.Int).Mul(l1GasUsed, new(big.Int).Add(scaledBaseFee, scaledBlobBaseFee)), big.NewInt(divisor))

	if !l1DataCost.IsUint64() {
		return ctx, fmt.Errorf("l1 data cost overflow: %s", l1DataCost)
	}

	ctx.GasMeter().ConsumeGas(l1DataCost.Uint64(), "l1 data")

	return ctx, nil
}

// getL1GasUsed calculates the compressed size of a transaction when encoded with RLP. This is used to estimate
// the gas cost of a transaction being posted to the L1 chain by the sequencer.
func getL1GasUsed(tx sdk.Tx) (*big.Int, error) {
	txBz, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to rlp encode tx: %w", err)
	}

	const zeroCost = 4
	const oneCost = 16
	const signatureCost = 68 * 16

	// l1GasUsed calculation: l1GasUsed = (zeroes * 4 + ones * 16) + signatureCost
	l1GasUsed := big.NewInt(signatureCost)
	for _, b := range txBz {
		if b == 0 {
			l1GasUsed.Add(l1GasUsed, big.NewInt(zeroCost))
		} else {
			l1GasUsed.Add(l1GasUsed, big.NewInt(oneCost))
		}
	}

	return l1GasUsed, nil
}
