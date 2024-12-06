package keeper

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/x/rollup/types"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SetL1Attributes(ctx context.Context, msg *types.MsgSetL1Attributes) (*types.MsgSetL1AttributesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := k.SetL1BlockInfo(sdkCtx, *msg.L1BlockInfo); err != nil {
		return nil, fmt.Errorf("set l1 block info: %v", err)
	}
	return &types.MsgSetL1AttributesResponse{}, nil
}

func (k *Keeper) ApplyUserDeposit(goCtx context.Context, msg *types.MsgApplyUserDeposit) (*types.MsgApplyUserDepositResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// L1BlockInfo is set in SetL1Attributes at the top of the block.
	l1blockInfo, err := k.GetL1BlockInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get l1 block info: %v", err)
	}

	// process L1 user deposit txs
	mintEvents, err := k.processL1UserDepositTxs(ctx, msg, l1blockInfo)
	if err != nil {
		return nil, types.WrapError(types.ErrProcessL1UserDepositTxs, "err: %v", err)
	}

	k.EmitEvents(goCtx, mintEvents)

	return &types.MsgApplyUserDepositResponse{}, nil
}

func (k *Keeper) InitiateWithdrawal(
	goCtx context.Context,
	msg *types.MsgInitiateWithdrawal,
) (*types.MsgInitiateWithdrawalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.Logger().Debug("Withdrawing ETH L2 assets", "sender", msg.Sender, "value", msg.Value)

	cosmAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, types.WrapError(types.ErrInvalidSender, "failed to create cosmos address for sender: %v; error: %v", msg.Sender, err)
	}

	if err = k.burnETH(ctx, cosmAddr, msg.Value); err != nil {
		return nil, types.WrapError(types.ErrBurnETH, "failed to burn ETH for cosmosAddress: %v; err: %v", cosmAddr, err)
	}

	withdrawalValueHex := hexutil.EncodeBig(msg.Value.BigInt())
	k.EmitEvents(ctx, sdk.Events{
		sdk.NewEvent(
			types.EventTypeWithdrawalInitiated,
			sdk.NewAttribute(types.AttributeKeySender, msg.Sender),
			sdk.NewAttribute(types.AttributeKeyL1Target, msg.Target),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
			sdk.NewAttribute(types.AttributeKeyGasLimit, hexutil.EncodeBig(new(big.Int).SetBytes(msg.GasLimit))),
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

func (k *Keeper) InitiateERC20Withdrawal(
	goCtx context.Context,
	msg *types.MsgInitiateERC20Withdrawal,
) (*types.MsgInitiateERC20WithdrawalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.Logger().Debug("Withdrawing ERC-20 L2 assets", "sender", msg.Sender, "value", msg.Value, "tokenAddress", msg.TokenAddress)

	cosmAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, types.WrapError(types.ErrInvalidSender, "failed to create cosmos address for sender: %v; error: %v", msg.Sender, err)
	}

	if err = k.burnERC20(ctx, cosmAddr, msg.TokenAddress, msg.Value); err != nil {
		return nil, types.WrapError(
			types.ErrInitiateERC20Withdrawal,
			"failed to burn ERC-20 for cosmosAddress: %v, tokenAddress: %v; err: %v",
			cosmAddr,
			msg.TokenAddress,
			err,
		)
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateERC20Withdrawal, "failed to get params: %v", err)
	}

	// Pack the finalizeBridgeERC20 message to forward to the L1StandardBridge contract on Ethereum.
	// see https://github.com/ethereum-optimism/optimism/blob/24a8d3e/packages/contracts-bedrock/src/universal/StandardBridge.sol#L267
	standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.StandardBridgeMetaData.ABI))
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateERC20Withdrawal, "failed to parse StandardBridge ABI: %v", err)
	}
	finalizeBridgeERC20Bz, err := standardBridgeABI.Pack(
		"finalizeBridgeERC20",
		common.HexToAddress(msg.TokenAddress), // local token
		common.HexToAddress(msg.TokenAddress), // remote token
		common.HexToAddress(msg.Sender),       // from
		common.HexToAddress(msg.Target),       // to
		msg.Value.BigInt(),                    // amount
		msg.ExtraData,                         // extra data
	)
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateERC20Withdrawal, "failed to pack finalizeBridgeERC20: %v", err)
	}

	// Pack the relayMessage to forward to the L1CrossDomainMessenger contract on Ethereum.
	// see https://github.com/ethereum-optimism/optimism/blob/24a8d3e/packages/contracts-bedrock/src/universal/CrossDomainMessenger.sol#L207
	crossDomainMessengerABI, err := abi.JSON(strings.NewReader(opbindings.CrossDomainMessengerMetaData.ABI))
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateERC20Withdrawal, "failed to parse CrossDomainMessenger ABI: %v", err)
	}
	relayMessageBz, err := crossDomainMessengerABI.Pack(
		"relayMessage",
		big.NewInt(0), // nonce
		common.HexToAddress(predeploys.L2StandardBridge), // sender
		common.HexToAddress(params.L1StandardBridge),     // target
		big.NewInt(0),         // value
		big.NewInt(0),         // min gas limit
		finalizeBridgeERC20Bz, // message
	)
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateERC20Withdrawal, "failed to pack relayMessage: %v", err)
	}

	withdrawalValueHex := hexutil.EncodeBig(msg.Value.BigInt())
	k.EmitEvents(ctx, sdk.Events{
		sdk.NewEvent(
			types.EventTypeWithdrawalInitiated,
			// To forward the ERC-20 withdrawal to L1, we need to use the L2CrossDomainMessenger address as the msg.sender
			sdk.NewAttribute(types.AttributeKeySender, predeploys.L2CrossDomainMessenger),
			sdk.NewAttribute(types.AttributeKeyL1Target, params.L1CrossDomainMessenger),
			// The ERC-20 withdrawal value is stored in the data field
			sdk.NewAttribute(types.AttributeKeyValue, hexutil.EncodeBig(big.NewInt(0))),
			sdk.NewAttribute(types.AttributeKeyGasLimit, hexutil.EncodeBig(new(big.Int).SetBytes(msg.GasLimit))),
			sdk.NewAttribute(types.AttributeKeyData, hexutil.Encode(relayMessageBz)),
			// The nonce attribute will be set by Monomer
		),
		sdk.NewEvent(
			types.EventTypeBurnERC20,
			sdk.NewAttribute(types.AttributeKeyL2WithdrawalTx, types.EventTypeWithdrawalInitiated),
			sdk.NewAttribute(types.AttributeKeyFromCosmosAddress, msg.Sender),
			sdk.NewAttribute(types.AttributeKeyERC20Address, msg.TokenAddress),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
		),
	})

	return &types.MsgInitiateERC20WithdrawalResponse{}, nil
}

func (k *Keeper) InitiateFeeWithdrawal(
	goCtx context.Context,
	_ *types.MsgInitiateFeeWithdrawal,
) (*types.MsgInitiateFeeWithdrawalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrInitiateFeeWithdrawal, "failed to get params: %v", err)
	}
	l1recipientAddr := params.L1FeeRecipient

	feeCollectorAddr := k.accountkeeper.GetModuleAddress(authtypes.FeeCollectorName)
	if feeCollectorAddr == nil {
		return nil, types.WrapError(types.ErrInitiateFeeWithdrawal, "failed to get fee collector address")
	}

	feeCollectorBalance := k.bankkeeper.GetBalance(ctx, feeCollectorAddr, types.WEI)
	if feeCollectorBalance.Amount.LT(math.NewIntFromUint64(params.MinFeeWithdrawalAmount)) {
		return nil, types.WrapError(
			types.ErrInitiateFeeWithdrawal,
			"fee collector balance is below the minimum withdrawal amount: (balance: %v, min withdrawal amount: %v)",
			feeCollectorBalance.String(),
			params.MinFeeWithdrawalAmount,
		)
	}

	ctx.Logger().Debug("Withdrawing L2 fees", "amount", feeCollectorBalance.String(), "recipient", l1recipientAddr)

	// Burn the withdrawn fees from the fee collector account on L2. To avoid needing to enable burn permissions for the
	// FeeCollector module account, they will first be sent to the rollup module account before being burned.
	fees := sdk.NewCoins(feeCollectorBalance)
	if err := k.bankkeeper.SendCoinsFromModuleToModule(ctx, authtypes.FeeCollectorName, types.ModuleName, fees); err != nil {
		return nil, types.WrapError(
			types.ErrInitiateFeeWithdrawal,
			"failed to send withdrawn fees from fee collector account to rollup module: %v", err,
		)
	}
	if err := k.bankkeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(feeCollectorBalance)); err != nil {
		return nil, types.WrapError(types.ErrInitiateFeeWithdrawal, "failed to burn withdrawn fees from rollup module: %v", err)
	}

	withdrawalValueHex := hexutil.EncodeBig(feeCollectorBalance.Amount.BigInt())
	feeCollectorAddrStr := feeCollectorAddr.String()
	k.EmitEvents(ctx, sdk.Events{
		sdk.NewEvent(
			types.EventTypeWithdrawalInitiated,
			sdk.NewAttribute(types.AttributeKeySender, feeCollectorAddrStr),
			sdk.NewAttribute(types.AttributeKeyL1Target, l1recipientAddr),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
			sdk.NewAttribute(types.AttributeKeyGasLimit, hexutil.EncodeBig(new(big.Int).SetUint64(params.FeeWithdrawalGasLimit))),
			sdk.NewAttribute(types.AttributeKeyData, "0x"),
			// The nonce attribute will be set by Monomer
		),
		sdk.NewEvent(
			types.EventTypeBurnETH,
			sdk.NewAttribute(types.AttributeKeyL2FeeWithdrawalTx, types.EventTypeWithdrawalInitiated),
			sdk.NewAttribute(types.AttributeKeyFromCosmosAddress, feeCollectorAddrStr),
			sdk.NewAttribute(types.AttributeKeyValue, withdrawalValueHex),
		),
	})

	return &types.MsgInitiateFeeWithdrawalResponse{}, nil
}

func (k *Keeper) UpdateParams(
	goCtx context.Context,
	msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if k.authority.String() != msg.Authority {
		return nil, types.WrapError(types.ErrUpdateParams, "invalid authority: expected %s, got %s", k.authority.String(), msg.Authority)
	}

	params := &msg.Params
	if err := params.Validate(); err != nil {
		return nil, types.WrapError(types.ErrUpdateParams, "validate params: %w", err)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := k.SetParams(ctx, &msg.Params); err != nil {
		return nil, types.WrapError(types.ErrUpdateParams, "set params: %w", err)
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
