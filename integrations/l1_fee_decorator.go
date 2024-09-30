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

	l1Fee, err := lfd.CalculateL1FeeEcotone(&ctx, txBytes)
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
func (lfd *L1FeeDecorator) CalculateL1FeeBedrock(ctx *sdk.Context, txBytes []byte) (sdk.Coin, error) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	store := ctx.KVStore(storeKey)
	l1BlockInfoBz := store.Get([]byte(types.KeyL1BlockInfo))
	if l1BlockInfoBz == nil {
		return sdk.Coin{}, sdkerrors.Wrap(errors.ErrNotFound, "L1 block info not found")
	}

	var l1BlockInfo types.L1BlockInfo
	if err := l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return sdk.Coin{}, err
	}

	/*
		Bedrock L1 Data Fee Formula: https://docs.optimism.io/stack/transactions/fees#bedrock

		1. Calculate the gas cost of the transaction data:
		   tx_data_gas = count_zero_bytes(tx_data) * 4 + count_non_zero_bytes(tx_data) * 16

		2. Apply the fixed and dynamic overhead costs:
		   tx_total_gas = (tx_data_gas + fixed_overhead) * dynamic_overhead

		3. Calculate the total L1 data fee:
			l1_data_fee = tx_total_gas * ethereum_base_fee
	*/

	var (
		l1BaseFee = new(big.Int).SetBytes(l1BlockInfo.BaseFee)
		l1DataFee = new(big.Float).SetInt(l1BaseFee)

		fixedOverheadBig   = big.NewInt(188)
		dynamicOverheadBig = big.NewFloat(0.684)
	)

	zeroBytes, nonZeroBytes := countZerosAndOnes(txBytes)
	txDataGas := float64(zeroBytes*4 + nonZeroBytes*16)

	totalGas := big.NewFloat(txDataGas)
	totalGas.Add(totalGas, big.NewFloat(float64(fixedOverheadBig.Int64())))
	totalGas.Mul(totalGas, dynamicOverheadBig)

	l1DataFee.Mul(totalGas, l1DataFee)

	l1Cost := new(big.Int)
	l1DataFee.Int(l1Cost)

	return sdk.NewCoin("ETH", math.NewIntFromBigInt(l1Cost)), nil
}

// CalculateL1FeeEcotone calculates the L1 fee for a given transaction
func (lfd *L1FeeDecorator) CalculateL1FeeEcotone(ctx *sdk.Context, txBytes []byte) (sdk.Coin, error) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	store := ctx.KVStore(storeKey)
	l1BlockInfoBz := store.Get([]byte(types.KeyL1BlockInfo))
	if l1BlockInfoBz == nil {
		return sdk.Coin{}, sdkerrors.Wrap(errors.ErrNotFound, "L1 block info not found")
	}

	var l1BlockInfo types.L1BlockInfo
	if err := l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return sdk.Coin{}, err
	}

	/*
		   Ecotone L1 Data Fee Formula: https://docs.optimism.io/stack/transactions/fees#ecotone

		   1. Estimate compressed transaction size:
		      tx_compressed_size = [(count_zero_bytes(tx_data)*4 + count_non_zero_bytes(tx_data)*16)] / 16

		   2. Calculate weighted gas price multiplier:
		      weighted_gas_price = 16 * base_fee_scalar * base_fee + blob_base_fee_scalar * blob_base_fee

		   3. Final L1 Data Fee:
		      l1_data_fee = tx_compressed_size * weighted_gas_price

		txBytes should be the signed transaction serialized according to the standard Ethereum transaction RLP encoding.
	*/

	var (
		l1BaseFee           = new(big.Int).SetBytes(l1BlockInfo.BaseFee)
		l1BaseFeeScalar     = new(big.Int).SetBytes(l1BlockInfo.L1FeeScalar)
		l1BlobBaseFee       = new(big.Int).SetBytes(l1BlockInfo.BlobBaseFee)
		l1BlobBaseFeeScalar = big.NewInt(int64(l1BlockInfo.BlobBaseFeeScalar))
	)

	zeroBytes, nonZeroBytes := countZerosAndOnes(txBytes)

	txCompressedSize := int64((zeroBytes*4 + nonZeroBytes*16) / 16)

	weightedGasPrice := new(big.Int).Mul(l1BaseFeeScalar, l1BaseFee)
	weightedGasPrice.Mul(weightedGasPrice, big.NewInt(16))
	blobBaseFeeComponent := new(big.Int).Mul(l1BlobBaseFeeScalar, l1BlobBaseFee)
	weightedGasPrice.Add(weightedGasPrice, blobBaseFeeComponent)

	txCompressedSizeBig := big.NewInt(txCompressedSize)
	l1Cost := new(big.Int).Mul(txCompressedSizeBig, weightedGasPrice)

	return sdk.NewCoin("ETH", math.NewIntFromBigInt(l1Cost)), nil
}

// CalculateL1FeeFjord calculates the L1 fee for a given transaction.
func (lfd *L1FeeDecorator) CalculateL1FeeFjord(ctx *sdk.Context, txBytes []byte) (sdk.Coin, error) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	store := ctx.KVStore(storeKey)

	// Retrieve L1 block info
	l1BlockInfoBz := store.Get([]byte(types.KeyL1BlockInfo))
	if l1BlockInfoBz == nil {
		return sdk.Coin{}, sdkerrors.Wrap(errors.ErrNotFound, "L1 block info not found")
	}

	var l1BlockInfo types.L1BlockInfo
	if err := l1BlockInfo.Unmarshal(l1BlockInfoBz); err != nil {
		return sdk.Coin{}, err
	}

	/*
		Fjord L1 Data Fee Formula: https://docs.optimism.io/stack/transactions/fees#fjord

		1. Estimate transaction size using FastLZ compression:
		   estimatedSizeScaled = max(minTransactionSize * 1e6, intercept + fastlzCoef * fastlzSize)

		2. Calculate weighted gas price multiplier:
		   l1FeeScaled = baseFeeScalar * l1BaseFee * 16 + blobFeeScalar * l1BlobBaseFee

		3. Final L1 Data Fee:
		   l1Cost = estimatedSizeScaled * l1FeeScaled / 1e12
	*/
	var (
		l1BaseFee           = new(big.Int).SetBytes(l1BlockInfo.BaseFee)
		l1BaseFeeScalar     = new(big.Int).SetBytes(l1BlockInfo.L1FeeScalar)
		l1BlobBaseFee       = new(big.Int).SetBytes(l1BlockInfo.BlobBaseFee)
		l1BlobBaseFeeScalar = big.NewInt(int64(l1BlockInfo.BlobBaseFeeScalar))
	)

	// Compression-related constants
	// This is from https://github.com/ethereum-optimism/op-geth/blob/d5a96613c22bc46238a21d6c0f805399c26c9d4c/core/types/rollup_cost.go#L78
	// Our dependency is not up to date, so this can't be imported
	const (
		minTransactionSize = 100
		intercept          = -42_585_600
		fastlzCoef         = 836_500
	)

	// Compress the transaction and estimate the scaled size
	fastlzSize := int(FlzCompressLen(txBytes))
	estimatedSizeScaled := max(minTransactionSize*1e6, intercept+fastlzCoef*fastlzSize)

	// Calculate the weighted gas price multiplier
	l1FeeScaled := l1BaseFeeScalar.Uint64()*l1BaseFee.Uint64()*16 +
		l1BlobBaseFeeScalar.Uint64()*l1BlobBaseFee.Uint64()

	// Calculate the final L1 Data Fee
	l1Cost := estimatedSizeScaled * int(l1FeeScaled/1e12)

	// Return the L1 fee as a new coin
	return sdk.NewCoin("ETH", math.NewInt(int64(l1Cost))), nil
}

// CollectL1Fee collects the L1 fee and sends it to the L1FeeCollectorAccount
func (lfd *L1FeeDecorator) CollectL1Fee(ctx *sdk.Context, feePayer sdk.AccAddress, fee sdk.Coin) error {
	if fee.IsZero() {
		return sdkerrors.Wrapf(errors.ErrInsufficientFee, "failed to deduct L1 fee: no fee amount specified")
	}
	// Todo: verify if the following approach is correct
	err := lfd.bankKeeper.SendCoinsFromAccountToModule(*ctx, feePayer, types.ModuleName, sdk.NewCoins(fee))
	if err != nil {
		return sdkerrors.Wrapf(errors.ErrInsufficientFunds, "failed to deduct L1 fee: %s", err)
	}
	err = lfd.bankKeeper.SendCoinsFromModuleToAccount(*ctx, types.ModuleName, types.L1FeeVaultAddress, sdk.NewCoins(fee))
	if err != nil {
		return sdkerrors.Wrapf(errors.ErrInsufficientFunds, "failed to deduct L1 fee: %s", err)
	}
	return nil
}

func countZerosAndOnes(data []byte) (zeros, ones int) {
	for _, b := range data {
		if b == 0 {
			zeros++
		} else {
			ones++
		}
	}
	return
}

// FlzCompressLen returns the length of the data after compression through FastLZ, based on
// https://github.com/Vectorized/solady/blob/5315d937d79b335c668896d7533ac603adac5315/js/solady.js
//
// This is from https://github.com/ethereum-optimism/op-geth/blob/d5a96613c22bc46238a21d6c0f805399c26c9d4c/core/types/rollup_cost.go#L399
// Our dependency is not up to date, so this can't be imported
func FlzCompressLen(ib []byte) uint32 {
	n := uint32(0)
	ht := make([]uint32, 8192)
	u24 := func(i uint32) uint32 {
		return uint32(ib[i]) | (uint32(ib[i+1]) << 8) | (uint32(ib[i+2]) << 16)
	}
	cmp := func(p uint32, q uint32, e uint32) uint32 {
		l := uint32(0)
		for e -= q; l < e; l++ {
			if ib[p+l] != ib[q+l] {
				e = 0
			}
		}
		return l
	}
	literals := func(r uint32) {
		n += 0x21 * (r / 0x20)
		r %= 0x20
		if r != 0 {
			n += r + 1
		}
	}
	match := func(l uint32) {
		l--
		n += 3 * (l / 262)
		if l%262 >= 6 {
			n += 3
		} else {
			n += 2
		}
	}
	hash := func(v uint32) uint32 {
		return ((2654435769 * v) >> 19) & 0x1fff
	}
	setNextHash := func(ip uint32) uint32 {
		ht[hash(u24(ip))] = ip
		return ip + 1
	}
	a := uint32(0)
	ipLimit := uint32(len(ib)) - 13
	if len(ib) < 13 {
		ipLimit = 0
	}
	for ip := a + 2; ip < ipLimit; {
		r := uint32(0)
		d := uint32(0)
		for {
			s := u24(ip)
			h := hash(s)
			r = ht[h]
			ht[h] = ip
			d = ip - r
			if ip >= ipLimit {
				break
			}
			ip++
			if d <= 0x1fff && s == u24(r) {
				break
			}
		}
		if ip >= ipLimit {
			break
		}
		ip--
		if ip > a {
			literals(ip - a)
		}
		l := cmp(r+3, ip+3, ipLimit+9)
		match(l)
		ip = setNextHash(setNextHash(ip + l))
		a = ip
	}
	literals(uint32(len(ib)) - a)
	return n
}
