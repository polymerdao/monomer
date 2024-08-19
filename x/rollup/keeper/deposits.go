package keeper

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	if err := k.storeService.OpenKVStore(ctx).Set([]byte(types.KeyL1BlockInfo), infoBytes); err != nil {
		return types.WrapError(err, "set")
	}
	return nil
}

// setL1BlockHistory sets the L1 block info to the app state, with the key being the blockhash, so we can look it up easily later.
func (k *Keeper) setL1BlockHistory(ctx context.Context, info *derive.L1BlockInfo) error {
	infoBytes, err := json.Marshal(info)
	if err != nil {
		return types.WrapError(err, "marshal L1 block info")
	}
	if err := k.storeService.OpenKVStore(ctx).Set(info.BlockHash.Bytes(), infoBytes); err != nil {
		return types.WrapError(err, "set")
	}
	return nil
}

// processL1SystemDepositTx processes the L1 Attributes deposit tx and returns the L1 block info.
func (k *Keeper) processL1SystemDepositTx(ctx sdk.Context, txBytes []byte) (*derive.L1BlockInfo, error) { //nolint:gocritic // hugeParam
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		ctx.Logger().Error("Failed to unmarshal system deposit transaction", "index", 0, "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal system deposit transaction: %v", err)
	}
	if !tx.IsDepositTx() {
		ctx.Logger().Error("First L1 tx must be a system deposit tx", "type", tx.Type())
		return nil, types.WrapError(types.ErrInvalidL1Txs, "first L1 tx must be a system deposit tx, but got type %d", tx.Type())
	}
	l1blockInfo, err := derive.L1BlockInfoFromBytes(k.rollupCfg, 0, tx.Data())
	if err != nil {
		ctx.Logger().Error("Failed to derive L1 block info from L1 Info Deposit tx", "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to derive L1 block info from L1 Info Deposit tx: %v", err)
	}
	return l1blockInfo, nil
}

// processL1UserDepositTxs processes the L1 user deposit txs and mints ETH to the user's cosmos address.
func (k *Keeper) processL1UserDepositTxs(ctx sdk.Context, txs [][]byte) error { //nolint:gocritic // hugeParam
	for i := 1; i < len(txs); i++ {
		txBytes := txs[i]
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			ctx.Logger().Error("Failed to unmarshal user deposit transaction", "index", i, "err", err, "txBytes", txBytes)
			return types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal user deposit transaction", "index", i, "err", err)
		}
		if !tx.IsDepositTx() {
			ctx.Logger().Error("L1 tx must be a user deposit tx", "index", i, "type", tx.Type())
			return types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, index:%d, type:%d", i, tx.Type())
		}
		if tx.IsSystemTx() {
			ctx.Logger().Error("L1 tx must be a user deposit tx", "type", tx.Type())
			return types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, type %d", tx.Type())
		}
		ctx.Logger().Debug("User deposit tx", "index", i, "tx", string(lo.Must(tx.MarshalJSON())))
		to := tx.To()
		// if the receipient is nil, it means the tx is creating a contract which we don't support, so return an error.
		// see https://github.com/ethereum-optimism/op-geth/blob/v1.101301.0-rc.2/core/state_processor.go#L154
		if to == nil {
			ctx.Logger().Error("Contract creation txs are not supported", "index", i)
			return types.WrapError(types.ErrInvalidL1Txs, "Contract creation txs are not supported, index:%d", i)
		}
		cosmAddr := evmToCosmos(*to)
		mintAmount := sdkmath.NewIntFromBigInt(tx.Value())
		err := k.mintETH(ctx, cosmAddr, mintAmount)
		if err != nil {
			ctx.Logger().Error("Failed to mint ETH", "evmAddress", to, "cosmosAddress", cosmAddr, "err", err)
			return types.WrapError(types.ErrMintETH, "failed to mint ETH for cosmosAddress: %v; err: %v", cosmAddr, err)
		}
	}
	return nil
}

// mintETH mints ETH to an account where the amount is in wei.
func (k *Keeper) mintETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) error { //nolint:gocritic // hugeParam
	coin := sdk.NewCoin(types.ETH, amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to mint deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to send deposit coins from rollup module to user account %v: %v", addr, err)
	}
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.ModuleName),
		),
		sdk.NewEvent(
			types.EventTypeMintETH,
			sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
			sdk.NewAttribute(types.AttributeKeyToCosmosAddress, addr.String()),
			sdk.NewAttribute(types.AttributeKeyValue, hexutil.Encode(amount.BigInt().Bytes())),
		),
	})
	return nil
}

// evmToCosmos converts an EVM address to a sdk.AccAddress
func evmToCosmos(addr common.Address) sdk.AccAddress {
	return addr.Bytes()
}
