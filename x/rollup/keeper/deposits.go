package keeper

import (
	"encoding/json"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer/utils"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

// setL1BlockInfo sets the L1 block info to the app state
//
// Persisted data conforms to optimism specs on L1 attributes:
// https://github.com/ethereum-optimism/optimism/blob/develop/specs/deposits.md#l1-attributes-predeployed-contract
func (k *Keeper) setL1BlockInfo(ctx sdk.Context, info derive.L1BlockInfo) error { //nolint:gocritic
	infoBytes, err := json.Marshal(info)
	if err != nil {
		return types.WrapError(err, "marshal L1 block info")
	}
	if err = k.storeService.OpenKVStore(ctx).Set([]byte(types.KeyL1BlockInfo), infoBytes); err != nil {
		return types.WrapError(err, "set latest L1 block info")
	}
	return nil
}

// processL1AttributesTx processes the L1 Attributes tx and returns the L1 block info.
func (k *Keeper) processL1AttributesTx(ctx sdk.Context, txBytes []byte) (*derive.L1BlockInfo, error) { //nolint:gocritic // hugeParam
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		ctx.Logger().Error("Failed to unmarshal L1 attributes transaction", "index", 0, "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal L1 attributes transaction: %v", err)
	}
	if !tx.IsDepositTx() {
		ctx.Logger().Error("First L1 tx must be a L1 attributes tx", "type", tx.Type())
		return nil, types.WrapError(types.ErrInvalidL1Txs, "first L1 tx must be a L1 attributes tx, but got type %d", tx.Type())
	}
	l1blockInfo, err := derive.L1BlockInfoFromBytes(k.rollupCfg, uint64(ctx.BlockTime().Unix()), tx.Data())
	if err != nil {
		ctx.Logger().Error("Failed to derive L1 block info from L1 Info Deposit tx", "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to derive L1 block info from L1 Info Deposit tx: %v", err)
	}
	return l1blockInfo, nil
}

// processL1UserDepositTxs processes the L1 user deposit txs, mints ETH to the user's cosmos address,
// and returns associated events.
func (k *Keeper) processL1UserDepositTxs(ctx sdk.Context, txs [][]byte) (sdk.Events, error) { //nolint:gocritic // hugeParam
	mintEvents := sdk.Events{}

	// skip the first tx - it is the L1 attributes tx
	for i := 1; i < len(txs); i++ {
		txBytes := txs[i]
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			ctx.Logger().Error("Failed to unmarshal user deposit transaction", "index", i, "err", err, "txBytes", txBytes)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal user deposit transaction", "index", i, "err", err)
		}
		if !tx.IsDepositTx() {
			ctx.Logger().Error("L1 tx must be a user deposit tx", "index", i, "type", tx.Type())
			return nil, types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, index:%d, type:%d", i, tx.Type())
		}
		if tx.IsSystemTx() {
			ctx.Logger().Error("L1 tx must be a user deposit tx", "type", tx.Type())
			return nil, types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, type %d", tx.Type())
		}
		ctx.Logger().Debug("User deposit tx", "index", i, "tx", string(lo.Must(tx.MarshalJSON())))
		// if the receipient is nil, it means the tx is creating a contract which we don't support, so return an error.
		// see https://github.com/ethereum-optimism/op-geth/blob/v1.101301.0-rc.2/core/state_processor.go#L154
		if tx.To() == nil {
			ctx.Logger().Error("Contract creation txs are not supported", "index", i)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "Contract creation txs are not supported, index:%d", i)
		}

		// Get the sender's address from the transaction
		from, err := ethtypes.NewCancunSigner(tx.ChainId()).Sender(&tx)
		if err != nil {
			ctx.Logger().Error("Failed to get sender address", "evmAddress", from, "err", err)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to get sender address: %v", err)
		}
		cosmAddr := utils.EvmToCosmosAddress(from)
		mintAmount := sdkmath.NewIntFromBigInt(tx.Mint())

		mintEvent, err := k.mintETH(ctx, cosmAddr, mintAmount)
		if err != nil {
			ctx.Logger().Error("Failed to mint ETH", "evmAddress", from, "cosmosAddress", cosmAddr, "err", err)
			return nil, types.WrapError(types.ErrMintETH, "failed to mint ETH for cosmosAddress: %v; err: %v", cosmAddr, err)
		}
		mintEvents = append(mintEvents, *mintEvent)
	}

	return mintEvents, nil
}

// mintETH mints ETH to an account where the amount is in wei and returns the associated event.
func (k *Keeper) mintETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) (*sdk.Event, error) { //nolint:gocritic // hugeParam
	coin := sdk.NewCoin(types.ETH, amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to mint deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to send deposit coins from rollup module to user account %v: %v", addr, err)
	}

	mintEvent := sdk.NewEvent(
		types.EventTypeMintETH,
		sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
		sdk.NewAttribute(types.AttributeKeyToCosmosAddress, addr.String()),
		sdk.NewAttribute(types.AttributeKeyValue, hexutil.Encode(amount.BigInt().Bytes())),
	)

	return &mintEvent, nil
}
