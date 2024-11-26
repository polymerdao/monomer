package keeper

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"

	"cosmossdk.io/math"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SetL1Attributes(ctx context.Context, msg *types.MsgSetL1Attributes) (*types.MsgSetL1AttributesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := k.SetL1BlockInfo(sdkCtx, msg.L1BlockInfo); err != nil {
		return nil, fmt.Errorf("set l1 block info: %v", err)
	}
	return &types.MsgSetL1AttributesResponse{}, nil
}

// TODO mint needs to always succeed... perhaps we can set something in the MintResponse and panic in the builder?
func (k *Keeper) Mint(ctx context.Context, msg *types.MsgMint) (*types.MsgMintResponse, error) {
	to, err := ethAddressBytesToCosmosAddr(msg.To)
	if err != nil {
		return nil, err
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(
		ctx,
		types.ModuleName,
		to,
		sdk.NewCoins(sdk.NewCoin(types.WEI, math.NewInt(int64(msg.Amount)))), // TODO is the cast a problem?
	); err != nil {
		return nil, fmt.Errorf("send coins from module to account: %v", err)
	}
	// TODO emit Mint event
	return &types.MsgMintResponse{}, nil
}

func (k *Keeper) ForceInclude(ctx context.Context, msg *types.MsgForceInclude) (*types.MsgForceIncludeResponse, error) {
	from, err := ethAddressBytesToCosmosAddr(msg.From)
	if err != nil {
		return nil, err
	}
	to, err := ethAddressBytesToCosmosAddr(msg.To)
	if err != nil {
		return nil, err
	}
	if msg.Value > 0 {
		coins := sdk.NewCoins(sdk.NewCoin(types.WEI, math.NewInt(int64(msg.Value)))) // TODO is the cast a problem?
		if err := k.bankkeeper.SendCoinsFromAccountToModule(
			ctx,
			from,
			types.ModuleName,
			coins,
		); err != nil {
			return nil, fmt.Errorf("send coins from account to module: %v", err)
		}
		if err := k.bankkeeper.SendCoinsFromModuleToAccount(
			ctx,
			types.ModuleName,
			to,
			coins,
		); err != nil {
			return nil, fmt.Errorf("send coins from module to account: %v", err)
		}
		// TODO emit transfer event
	}
	// TODO check IsCreation, fail if true

	// process crossdomainmessage

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrParams, "failed to get params: %v", err)
	}

	// Convert the L1CrossDomainMessenger address to its L2 aliased address
	aliasedL1CrossDomainMessengerAddress := crossdomain.ApplyL1ToL2Alias(common.HexToAddress(params.L1CrossDomainMessenger))
	// Check if the tx is a cross domain message from the aliased L1CrossDomainMessenger address
	if common.Address(msg.From) == aliasedL1CrossDomainMessengerAddress && msg.Data != nil {
		// TODO: Investigate when to return an error if a cross domain message can't be parsed or executed - look at OP Spec

		crossDomainMessengerABI, err := abi.JSON(strings.NewReader(opbindings.CrossDomainMessengerMetaData.ABI))
		if err != nil {
			return nil, fmt.Errorf("failed to parse CrossDomainMessenger ABI: %v", err)
		}
		standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.StandardBridgeMetaData.ABI))
		if err != nil {
			return nil, fmt.Errorf("failed to parse StandardBridge ABI: %v", err)
		}

		var relayMessage bindings.RelayMessageArgs
		if err = unpackInputsIntoInterface(&crossDomainMessengerABI, "relayMessage", msg.Data, &relayMessage); err != nil {
			return nil, fmt.Errorf("failed to unpack tx data into relayMessage interface: %v", err)
		}

		// Check if the relayed message is a finalizeBridgeERC20 message from the L1StandardBridge
		if !bytes.Equal(relayMessage.Message[:4], standardBridgeABI.Methods["finalizeBridgeERC20"].ID) {
			return nil, fmt.Errorf("tx data not recognized as a cross domain message: %v", msg.Data)
		}

		var finalizeBridgeERC20 bindings.FinalizeBridgeERC20Args
		if err = unpackInputsIntoInterface(
			&standardBridgeABI,
			"finalizeBridgeERC20",
			relayMessage.Message,
			&finalizeBridgeERC20,
		); err != nil {
			return nil, fmt.Errorf("failed to unpack relay message into finalizeBridgeERC20 interface: %v", err)
		}

		// TODO properly process toAddr
		toAddr, err := monomer.CosmosETHAddress(finalizeBridgeERC20.To).Encode(sdk.GetConfig().GetBech32AccountAddrPrefix())
		if err != nil {
			return nil, fmt.Errorf("evm to cosmos address: %v", err)
		}
		// Mint the ERC-20 token to the specified Cosmos address
		erc20Addr := finalizeBridgeERC20.RemoteToken.String()
		coin := sdk.NewCoin("erc20/"+erc20Addr[2:], sdkmath.NewIntFromBigInt(finalizeBridgeERC20.Amount))
		coins := sdk.NewCoins(coin)
		if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("mint coins: %v", err)
		}
		if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, toAddr, coins); err != nil {
			return nil, fmt.Errorf("send coins from module to account: %v", err)
		}

		mintEvents = append(mintEvents, sdk.NewEvent(
			types.EventTypeMintERC20,
			sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
			sdk.NewAttribute(types.AttributeKeyToCosmosAddress, to.String()),
			sdk.NewAttribute(types.AttributeKeyERC20Address, erc20Addr),
			sdk.NewAttribute(types.AttributeKeyValue, hexutil.EncodeBig(coin.Amount.BigInt())),
		))
	}

	return &types.MsgForceIncludeResponse{}, nil
}

func ethAddressBytesToCosmosAddr(ethAddr []byte) (sdk.AccAddress, error) {
	cosmosAddrStr, err := monomer.CosmosETHAddress(ethAddr).Encode(sdk.GetConfig().GetBech32AccountAddrPrefix())
	if err != nil {
		return nil, fmt.Errorf("encode eth address to Cosmos bech32: %v", err)
	}
	cosmosAddr, err := sdk.AccAddressFromBech32(cosmosAddrStr)
	if err != nil {
		return nil, fmt.Errorf("account address from bech32: %v", err)
	}
	return cosmosAddr, nil
}

// ApplyL1Txs implements types.MsgServer.
func (k *Keeper) ApplyL1Txs(goCtx context.Context, msg *types.MsgApplyL1Txs) (*types.MsgApplyL1TxsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	ctx.Logger().Debug("Processing L1 txs", "txCount", len(msg.TxBytes))

	// process L1 attributes tx and get L1 block info
	l1blockInfo, err := k.processL1AttributesTx(ctx, msg.TxBytes[0])
	if err != nil {
		return nil, types.WrapError(types.ErrProcessL1SystemDepositTx, "err: %v", err)
	}

	// save L1 block info to AppState
	if err = k.SetL1BlockInfo(ctx, *l1blockInfo); err != nil {
		return nil, types.WrapError(types.ErrL1BlockInfo, "save error: %v", err)
	}

	ctx.Logger().Debug("Save L1 block info", "l1blockInfo", string(lo.Must(l1blockInfo.Marshal())))

	// process L1 user deposit txs
	mintEvents, err := k.processL1UserDepositTxs(ctx, msg.TxBytes, l1blockInfo)
	if err != nil {
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
