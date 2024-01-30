package keeper

/**
 Keeper methods here are not invoked by regular SDK tx msgs, but instead triggered by L1 txs on both sequencer and
verifier nodes without using a cosmos account.
*/

import (
	"context"
	"encoding/json"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer/app/peptide/common"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

type msgServer struct {
	*Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper *Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// ApplyL1Txs implements types.MsgServer.
func (k Keeper) ApplyL1Txs(goCtx context.Context, msg *types.MsgL1Txs) (*types.MsgL1TxsResponse, error) {
	if msg.TxBytes == nil || len(msg.TxBytes) < 1 {
		return nil, types.WrapError(types.ErrInvalidL1Txs, "must have at least one L1 Info Deposit tx")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	ctx.Logger().Debug("processing L1 txs", "txCount", len(msg.TxBytes))

	// process L1 system deposit tx
	txBytes := msg.TxBytes[0]
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		ctx.Logger().Error("failed to unmarshal system deposit transaction", "index", 0, "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal system deposit transaction: %v", err)
	}
	if !tx.IsDepositTx() {
		ctx.Logger().Error("first L1 tx must be a system deposit tx", "type", tx.Type())
		return nil, types.WrapError(types.ErrInvalidL1Txs, "first L1 tx must be a system deposit tx, but got type %d", tx.Type())
	}
	l1blockInfo, err := derive.L1BlockInfoFromBytes(k.rollupCfg, 0, tx.Data())

	if err != nil {
		ctx.Logger().Error("failed to derive L1 block info from L1 Info Deposit tx", "err", err, "txBytes", txBytes)
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to derive L1 block info from L1 Info Deposit tx: %v", err)
	}

	// save L1 block info to AppState
	if err := k.SetL1BlockInfo(k.rollupCfg, ctx, *l1blockInfo); err != nil {
		ctx.Logger().Error("failed to save L1 block info to AppState", "err", err)
		return nil, types.WrapError(types.ErrL1BlockInfo, "save error: %v", err)
	}

	ctx.Logger().Info("save L1 block info", "l1blockInfo", string(lo.Must(json.Marshal(l1blockInfo))))

	// save L1 block History to AppState
	if err := k.SetL1BlockHistory(k.rollupCfg, ctx, *l1blockInfo); err != nil {
		ctx.Logger().Error("failed to save L1 block history info to AppState", "err", err)
		return nil, types.WrapError(types.ErrL1BlockInfo, "save error: %v", err)
	}

	ctx.Logger().Info("save L1 block history info", "l1blockHistoryInfo", string(lo.Must(json.Marshal(l1blockInfo))))

	// process L1 user deposit txs
	for i := 1; i < len(msg.TxBytes); i++ {
		txBytes := msg.TxBytes[i]
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			ctx.Logger().Error("failed to unmarshal user deposit transaction", "index", i, "err", err, "txBytes", txBytes)
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
		ctx.Logger().Debug("user deposit tx", "index", i, "tx", string(lo.Must(tx.MarshalJSON())))
		to := tx.To()
		// if the receipient is nil, it means the tx is creating a contract which we don't support, so return an error.
		// see https://github.com/ethereum-optimism/op-geth/blob/v1.101301.0-rc.2/core/state_processor.go#L154
		if to == nil {
			ctx.Logger().Error("Contract creation txs are not supported", "index", i)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "Contract creation txs are not supported, index:%d", i)
		}
		cosmAddr := common.EvmToCosmos(*to)
		mintAmount := sdkmath.NewIntFromBigInt(tx.Value())
		err := k.MintETH(ctx, cosmAddr, mintAmount)
		if err != nil {
			ctx.Logger().Error("failed to mint ETH", "evmAddress", to, "polymerAddress", cosmAddr, "err", err)
			return nil, types.WrapError(types.ErrMintETH, "failed to mint ETH", "polymerAddress", cosmAddr, "err", err)
		}
	}
	return &types.MsgL1TxsResponse{}, nil
}

// MintETH mints ETH to an account where the amount is in wei, the smallest unit of ETH
func (k Keeper) MintETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) error {
	coin := sdk.NewCoin(types.ETH, amount)
	if err := k.mintKeeper.MintCoins(ctx, sdk.NewCoins(coin)); err != nil {
		return err
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.MintModule, addr, sdk.NewCoins(coin)); err != nil {
		return err
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
			sdk.NewAttribute(types.AttributeKeyAmount, hexutil.Encode((amount.BigInt().Bytes()))),
		),
	})
	return nil
}

// SetL1BlockInfo sets the L1 block info to the app state
//
// Persisted data conforms to optimism specs on L1 attributes:
// https://github.com/ethereum-optimism/optimism/blob/develop/specs/deposits.md#l1-attributes-predeployed-contract
func (k Keeper) SetL1BlockInfo(rollupCfg *rollup.Config, ctx sdk.Context, l1blockInfo derive.L1BlockInfo) error {
	bz, err := derive.L1BlockInfoToBytes(rollupCfg, 0, l1blockInfo)
	if err != nil {
		return types.WrapError(err, "failed to marshal L1 block info to binary bytes")
	}
	ctx.KVStore(k.storeKey).Set([]byte(types.KeyL1BlockInfo), bz)
	return nil
}

// GetL1BlockInfo gets the L1 block info from the app state
func (k Keeper) GetL1BlockInfo(rollupCfg *rollup.Config, ctx sdk.Context) (derive.L1BlockInfo, error) {
	bz := ctx.KVStore(k.storeKey).Get([]byte(types.KeyL1BlockInfo))
	if bz == nil {
		return derive.L1BlockInfo{}, types.WrapError(types.ErrL1BlockInfo, "not found")
	}

	if l1blockInfo, err := derive.L1BlockInfoFromBytes(rollupCfg, 0, bz); err != nil {
		return derive.L1BlockInfo{}, types.WrapError(err, "failed to unmarshal from binary bytes")
	} else {
		return *l1blockInfo, nil
	}
}

// SetL1BlockHistory sets the L1 block info to the app state, with the key being the blockhash, so we can look it up easily later.
func (k Keeper) SetL1BlockHistory(rollupCfg *rollup.Config, ctx sdk.Context, l1blockInfo derive.L1BlockInfo) error {
	bz, err := derive.L1BlockInfoToBytes(rollupCfg, 0, l1blockInfo)
	if err != nil {
		return types.WrapError(err, "failed to marshal L1 block info to binary bytes")
	}
	ctx.KVStore(k.storeKey).Set(l1blockInfo.BlockHash.Bytes(), bz)

	return nil
}

// GetL1BlockInfo retreieves the L1 block info from the app state
func (k Keeper) GetL1BlockHistory(rollupCfg *rollup.Config, ctx sdk.Context, blockHash []byte) (derive.L1BlockInfo, error) {
	bz := ctx.KVStore(k.storeKey).Get(blockHash)
	if bz == nil {
		return derive.L1BlockInfo{}, types.WrapError(types.ErrL1BlockInfo, "not found")
	}
	if l1blockInfo, err := derive.L1BlockInfoFromBytes(rollupCfg, 0, bz); err != nil {
		return derive.L1BlockInfo{}, types.WrapError(err, "failed to unmarshal from binary bytes")
	} else {
		return *l1blockInfo, nil
	}
}
