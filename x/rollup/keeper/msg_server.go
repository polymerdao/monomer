package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

var _ types.MsgServer = &Keeper{}

// ApplyL1Txs implements types.MsgServer.
func (k *Keeper) ApplyL1Txs(goCtx context.Context, msg *types.MsgApplyL1Txs) (*types.MsgApplyL1TxsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	ctx.Logger().Debug("Processing L1 txs", "txCount", len(msg.TxBytes))

	// process L1 attributes tx and get L1 block info
	l1blockInfo, err := k.processL1AttributesTx(ctx, msg.TxBytes[0])
	if err != nil {
		ctx.Logger().Error("Failed to process L1 system deposit tx", "err", err)
		return nil, types.WrapError(types.ErrProcessL1SystemDepositTx, "err: %v", err)
	}

	// save L1 block info to AppState
	if err = k.setL1BlockInfo(ctx, *l1blockInfo); err != nil {
		ctx.Logger().Error("Failed to save L1 block info to AppState", "err", err)
		return nil, types.WrapError(types.ErrL1BlockInfo, "save error: %v", err)
	}

	ctx.Logger().Info("Save L1 block info", "l1blockInfo", string(lo.Must(l1blockInfo.Marshal())))

	// process L1 user deposit txs
	mintEvents, err := k.processL1UserDepositTxs(ctx, msg.TxBytes, l1blockInfo)
	if err != nil {
		ctx.Logger().Error("Failed to process L1 user deposit txs", "err", err)
		return nil, types.WrapError(types.ErrProcessL1UserDepositTxs, "err: %v", err)
	}

	k.EmitEvents(goCtx, mintEvents)

	return &types.MsgApplyL1TxsResponse{}, nil
}

func (k *Keeper) InitiateWithdrawal(
	goCtx context.Context,
	msg *types.MsgInitiateWithdrawal,
) (*types.MsgInitiateWithdrawalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.Logger().Debug("Withdrawing L2 assets", "sender", msg.Sender, "value", msg.Value)

	cosmAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		ctx.Logger().Error("Invalid sender address", "sender", msg.Sender, "err", err)
		return nil, types.WrapError(types.ErrInvalidSender, "failed to create cosmos address for sender: %v; error: %v", msg.Sender, err)
	}

	if err = k.burnETH(ctx, cosmAddr, msg.Value); err != nil {
		ctx.Logger().Error("Failed to burn ETH", "cosmosAddress", cosmAddr, "evmAddress", msg.Target, "err", err)
		return nil, types.WrapError(types.ErrBurnETH, "failed to burn ETH for cosmosAddress: %v; err: %v", cosmAddr, err)
	}

	withdrawalValueHex := hexutil.Encode(msg.Value.BigInt().Bytes())
	k.EmitEvents(ctx, sdk.Events{
		sdk.NewEvent(
			types.EventTypeWithdrawalInitiated,
			sdk.NewAttribute(types.AttributeKeySender, msg.Sender),
			sdk.NewAttribute(types.AttributeKeyL1Target, msg.Target),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
			sdk.NewAttribute(types.AttributeKeyGasLimit, hexutil.Encode(msg.GasLimit)),
			sdk.NewAttribute(types.AttributeKeyData, hexutil.Encode(msg.Data)),
			// The nonce attribute will be set by Monomer
		),
		sdk.NewEvent(
			types.EventTypeBurnETH,
			sdk.NewAttribute(types.AttributeKeyL2WithdrawalTx, types.EventTypeWithdrawalInitiated),
			sdk.NewAttribute(types.AttributeKeyFromCosmosAddress, msg.Sender),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
		),
	})

	return &types.MsgInitiateWithdrawalResponse{}, nil
}
