package integrations

import (
	"math/big"

	sdkerrors "cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type L1FeeCalculator interface {
	GetLatestL1BlockInfo(ctx *sdk.Context) (types.L1BlockInfo, error)
	CollectL1Fee(ctx *sdk.Context, feePayer sdk.AccAddress, fee sdk.Coin) error
}

type L1FeeDecorator struct {
	bankKeeper types.BankKeeper
}

// Ensure L1FeeDecorator implements the sdk.AnteDecorator interface
var _ sdk.AnteDecorator = &L1FeeDecorator{}

func NewL1FeeDecorator(bankKeeper types.BankKeeper) L1FeeDecorator {
	return L1FeeDecorator{bankKeeper: bankKeeper}
}

// AnteHandle handles L1 fee deduction and validation
func (lfd *L1FeeDecorator) AnteHandle(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	tx sdk.Tx, simulate bool,
	next sdk.AnteHandler,
) (sdk.Context, error) {
	if simulate {
		return sdk.Context{}, nil
	}

	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return ctx, sdkerrors.Wrap(errors.ErrTxDecode, "Tx must be a FeeTx")
	}

	txBytes, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return ctx, sdkerrors.Wrap(errors.ErrTxDecode, "failed to serialize transaction")
	}

	l1Fee, err := lfd.CalculateL1Fee(&ctx, txBytes)
	if err != nil {
		return ctx, sdkerrors.Wrapf(errors.ErrInsufficientFee, "failed to calculate L1 fee: %s", err)
	}

	providedFee := feeTx.GetFee()
	requiredFee := sdk.NewCoins(l1Fee)

	if providedFee.IsAllLT(requiredFee) {
		return ctx, sdkerrors.Wrapf(errors.ErrInsufficientFee, "insufficient fee; got: %s required: %s", providedFee, requiredFee)
	}

	err = lfd.CollectL1Fee(&ctx, feeTx.FeePayer(), l1Fee)
	if err != nil {
		return ctx, sdkerrors.Wrapf(errors.ErrInsufficientFunds, "failed to collect L1 fee: %s", err)
	}

	return next(ctx, tx, simulate)
}

// CalculateL1FeeBedrock calculates the L1 fee for a given transaction
func (lfd *L1FeeDecorator) CalculateL1Fee(ctx *sdk.Context, txBytes []byte) (sdk.Coin, error) {
	store := ctx.KVStore(storetypes.NewKVStoreKey(types.StoreKey))
	l1BlockInfoBz := store.Get([]byte(types.KeyL1BlockInfo))
	if l1BlockInfoBz == nil {
		return sdk.Coin{}, sdkerrors.Wrap(errors.ErrNotFound, "L1 block info not found")
	}

	var l1BlockInfo types.L1BlockInfo
	if err := l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return sdk.Coin{}, err
	}

	rollupDataGas := calculateRollupDataGas(txBytes)

	l1FeeOverhead := new(big.Int).SetBytes(l1BlockInfo.L1FeeOverhead)
	l1FeeScalar := new(big.Int).SetBytes(l1BlockInfo.L1FeeScalar)
	l1BaseFee := new(big.Int).SetBytes(l1BlockInfo.BaseFee)

	// Pre-Ecotone L1 Data Fee Formula
	// (rollupDataGas + l1FeeOverhead) * l1BaseFee * l1FeeScalar / 1e6
	const divisor = 1e6
	totalFee := new(big.Int).Add(rollupDataGas, l1FeeOverhead)
	totalFee.Mul(totalFee, l1BaseFee)
	totalFee.Mul(totalFee, l1FeeScalar)
	totalFee.Div(totalFee, big.NewInt(divisor))

	return sdk.NewCoin(types.ETH, math.NewIntFromBigInt(totalFee)), nil
}

// CollectL1Fee collects the L1 fee and sends it to the L1FeeCollectorAccount
func (lfd *L1FeeDecorator) CollectL1Fee(ctx *sdk.Context, feePayer sdk.AccAddress, fee sdk.Coin) error {
	if fee.IsZero() {
		return sdkerrors.Wrapf(errors.ErrInsufficientFee, "failed to deduct L1 fee: no fee amount specified")
	}
	if err := lfd.bankKeeper.SendCoinsFromAccountToModule(*ctx, feePayer, types.ModuleName, sdk.NewCoins(fee)); err != nil {
		return sdkerrors.Wrapf(errors.ErrInsufficientFunds, "failed to deduct L1 fee: %s", err)
	}
	// Update the tracked collected L1 fees
	err := addCollectedL1Fees(ctx, fee)
	if err != nil {
		return err
	}
	return nil
}

func calculateRollupDataGas(txBytes []byte) *big.Int {
	zeros, ones := 0, 0
	for _, b := range txBytes {
		if b == 0 {
			zeros++
		} else {
			ones++
		}
	}
	return big.NewInt(int64(zeros*4 + ones*16))
}

// AddCollectedL1Fees adds to the total collected L1 fees
func addCollectedL1Fees(ctx *sdk.Context, amount sdk.Coin) error {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	store := ctx.KVStore(storeKey)
	feeBytes := store.Get([]byte(types.KeyCollectedFees))

	var fee sdk.Coin
	err := fee.Unmarshal(feeBytes)
	if err != nil {
		return types.WrapError(err, "add collected fee amount")
	}

	fee = fee.Add(amount)
	feeBytes, err = fee.Marshal()
	store.Set([]byte(""), feeBytes)
	if err != nil {
		return types.WrapError(err, "add collected fee amount")
	}
	return nil
}
