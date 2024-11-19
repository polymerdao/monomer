package keeper

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

// processL1AttributesTx processes the L1 Attributes tx and returns the L1 block info.
func (k *Keeper) processL1AttributesTx(ctx sdk.Context, txBytes []byte) (*types.L1BlockInfo, error) { //nolint:gocritic // hugeParam
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal L1 attributes transaction: %v", err)
	}
	if !tx.IsDepositTx() {
		return nil, types.WrapError(types.ErrInvalidL1Txs, "first L1 tx must be a L1 attributes tx, but got type %d", tx.Type())
	}

	l1blockInfo, err := derive.L1BlockInfoFromBytes(k.rollupCfg, uint64(ctx.BlockTime().Unix()), tx.Data())
	if err != nil {
		return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to derive L1 block info from L1 Info Deposit tx: %v", err)
	}

	// Convert derive.L1BlockInfo to types.L1BlockInfo
	protoL1BlockInfo := &types.L1BlockInfo{
		Number:            l1blockInfo.Number,
		Time:              l1blockInfo.Time,
		BlockHash:         l1blockInfo.BlockHash[:],
		SequenceNumber:    l1blockInfo.SequenceNumber,
		BatcherAddr:       l1blockInfo.BatcherAddr[:],
		L1FeeOverhead:     l1blockInfo.L1FeeOverhead[:],
		L1FeeScalar:       l1blockInfo.L1FeeScalar[:],
		BaseFeeScalar:     l1blockInfo.BaseFeeScalar,
		BlobBaseFeeScalar: l1blockInfo.BlobBaseFeeScalar,
	}

	if l1blockInfo.BaseFee != nil {
		protoL1BlockInfo.BaseFee = l1blockInfo.BaseFee.Bytes()
	}
	if l1blockInfo.BlobBaseFee != nil {
		protoL1BlockInfo.BlobBaseFee = l1blockInfo.BlobBaseFee.Bytes()
	}
	return protoL1BlockInfo, nil
}

// processL1UserDepositTxs processes the L1 user deposit txs, mints ETH to the user's cosmos address,
// and returns associated events.
func (k *Keeper) processL1UserDepositTxs(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	txs [][]byte,
	l1blockInfo *types.L1BlockInfo,
) (sdk.Events, error) {
	mintEvents := sdk.Events{}

	// skip the first tx - it is the L1 attributes tx
	for i := 1; i < len(txs); i++ {
		txBytes := txs[i]
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to unmarshal user deposit transaction", "index", i, "err", err)
		}
		if !tx.IsDepositTx() {
			return nil, types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, index:%d, type:%d", i, tx.Type())
		}
		if tx.IsSystemTx() {
			return nil, types.WrapError(types.ErrInvalidL1Txs, "L1 tx must be a user deposit tx, type %d", tx.Type())
		}
		ctx.Logger().Debug("User deposit tx", "index", i, "tx", string(lo.Must(tx.MarshalJSON())))
		// if the receipient is nil, it means the tx is creating a contract which we don't support, so return an error.
		// see https://github.com/ethereum-optimism/op-geth/blob/v1.101301.0-rc.2/core/state_processor.go#L154
		if tx.To() == nil {
			return nil, types.WrapError(types.ErrInvalidL1Txs, "Contract creation txs are not supported, index:%d", i)
		}

		// Get the sender's address from the transaction
		from, err := ethtypes.MakeSigner(
			monomer.NewChainConfig(tx.ChainId()),
			new(big.Int).SetUint64(l1blockInfo.Number),
			l1blockInfo.Time,
		).Sender(&tx)
		if err != nil {
			return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to get sender address: %v", err)
		}
		addrPrefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
		mintAddr, err := monomer.CosmosETHAddress(from).Encode(addrPrefix)
		if err != nil {
			return nil, fmt.Errorf("evm to cosmos address: %v", err)
		}
		mintAmount := sdkmath.NewIntFromBigInt(tx.Mint())
		recipientAddr, err := monomer.CosmosETHAddress(*tx.To()).Encode(addrPrefix)
		if err != nil {
			return nil, fmt.Errorf("evm to cosmos address: %v", err)
		}
		transferAmount := sdkmath.NewIntFromBigInt(tx.Value())

		mintEvent, err := k.mintETH(ctx, mintAddr, recipientAddr, mintAmount, transferAmount)
		if err != nil {
			return nil, types.WrapError(types.ErrMintETH, "failed to mint ETH for cosmosAddress: %v; err: %v", mintAddr, err)
		}
		mintEvents = append(mintEvents, *mintEvent)

		params, err := k.GetParams(ctx)
		if err != nil {
			return nil, types.WrapError(types.ErrParams, "failed to get params: %v", err)
		}

		// Convert the L1CrossDomainMessenger address to its L2 aliased address
		aliasedL1CrossDomainMessengerAddress := crossdomain.ApplyL1ToL2Alias(common.HexToAddress(params.L1CrossDomainMessenger))

		// Check if the tx is a cross domain message from the aliased L1CrossDomainMessenger address
		if from == aliasedL1CrossDomainMessengerAddress && tx.Data() != nil {
			crossDomainMessageEvent, err := k.processCrossDomainMessage(ctx, tx.Data())
			// TODO: Investigate when to return an error if a cross domain message can't be parsed or executed - look at OP Spec
			if err != nil {
				return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to parse or execute cross domain message: %v", err)
			} else if crossDomainMessageEvent != nil {
				mintEvents = append(mintEvents, *crossDomainMessageEvent)
			}
		}
	}

	return mintEvents, nil
}

// processCrossDomainMessage parses the tx data of a cross domain message and applies state transitions for recognized messages.
// Currently, only finalizeBridgeETH and finalizeBridgeERC20 messages from the L1StandardBridge are recognized for minting tokens
// on the Cosmos chain. If a message is not recognized, it returns nil and does not error.
func (k *Keeper) processCrossDomainMessage(ctx sdk.Context, txData []byte) (*sdk.Event, error) { //nolint:gocritic // hugeParam
	crossDomainMessengerABI, err := abi.JSON(strings.NewReader(opbindings.CrossDomainMessengerMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse CrossDomainMessenger ABI: %v", err)
	}
	standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.StandardBridgeMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse StandardBridge ABI: %v", err)
	}

	var relayMessage bindings.RelayMessageArgs
	if err = unpackInputsIntoInterface(&crossDomainMessengerABI, "relayMessage", txData, &relayMessage); err != nil {
		return nil, fmt.Errorf("failed to unpack tx data into relayMessage interface: %v", err)
	}

	// Check if the relayed message is a supported message from the L1StandardBridge
	l1StandardBridgeMethodID := relayMessage.Message[:4]
	if bytes.Equal(l1StandardBridgeMethodID, standardBridgeABI.Methods["finalizeBridgeETH"].ID) {
		mintEvent, err := k.processFinalizeBridgeETH(ctx, &standardBridgeABI, &relayMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to process finalizeBridgeETH method: %v", err)
		}
		return mintEvent, nil
	} else if bytes.Equal(l1StandardBridgeMethodID, standardBridgeABI.Methods["finalizeBridgeERC20"].ID) {
		mintEvent, err := k.processFinalizeBridgeERC20(ctx, &standardBridgeABI, &relayMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to process finalizeBridgeERC20 method: %v", err)
		}
		return mintEvent, nil
	}

	ctx.Logger().Debug("Unsupported cross domain message", "methodID", hexutil.Encode(l1StandardBridgeMethodID))
	return nil, nil
}

func (k *Keeper) processFinalizeBridgeETH(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	standardBridgeABI *abi.ABI,
	relayMessage *bindings.RelayMessageArgs,
) (*sdk.Event, error) {
	var finalizeBridgeETH bindings.FinalizeBridgeETHArgs
	if err := unpackInputsIntoInterface(standardBridgeABI, "finalizeBridgeETH", relayMessage.Message, &finalizeBridgeETH); err != nil {
		return nil, fmt.Errorf("failed to unpack relay message into finalizeBridgeETH interface: %v", err)
	}

	toAddr, err := monomer.CosmosETHAddress(finalizeBridgeETH.To).Encode(sdk.GetConfig().GetBech32AccountAddrPrefix())
	if err != nil {
		return nil, fmt.Errorf("evm to cosmos address: %v", err)
	}

	amount := sdkmath.NewIntFromBigInt(finalizeBridgeETH.Amount)
	mintEvent, err := k.mintETH(ctx, toAddr, toAddr, amount, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to mint ETH: %v", err)
	}

	return mintEvent, nil
}

func (k *Keeper) processFinalizeBridgeERC20(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	standardBridgeABI *abi.ABI,
	relayMessage *bindings.RelayMessageArgs,
) (*sdk.Event, error) {
	var finalizeBridgeERC20 bindings.FinalizeBridgeERC20Args
	if err := unpackInputsIntoInterface(standardBridgeABI, "finalizeBridgeERC20", relayMessage.Message, &finalizeBridgeERC20); err != nil {
		return nil, fmt.Errorf("failed to unpack relay message into finalizeBridgeERC20 interface: %v", err)
	}

	toAddr, err := monomer.CosmosETHAddress(finalizeBridgeERC20.To).Encode(sdk.GetConfig().GetBech32AccountAddrPrefix())
	if err != nil {
		return nil, fmt.Errorf("evm to cosmos address: %v", err)
	}

	mintEvent, err := k.mintERC20(ctx, toAddr, finalizeBridgeERC20.RemoteToken.String(), sdkmath.NewIntFromBigInt(finalizeBridgeERC20.Amount))
	if err != nil {
		return nil, fmt.Errorf("failed to mint ERC-20 token: %v", err)
	}

	return mintEvent, nil
}

// mintETH mints ETH to an account where the amount is in wei and returns the associated event.
func (k *Keeper) mintETH(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	mintAddr, recipientAddr string,
	mintAmount, transferAmount sdkmath.Int,
) (*sdk.Event, error) {
	// Mint the deposit amount to the rollup module
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(sdk.NewCoin(types.WEI, mintAmount))); err != nil {
		return nil, fmt.Errorf("failed to mint ETH deposit coins to the rollup module: %v", err)
	}

	if transferAmount.GT(mintAmount) {
		return nil, fmt.Errorf("transfer amount %v is greater than mint amount %v", transferAmount, mintAmount)
	}

	recipientSDKAddr, err := sdk.AccAddressFromBech32(recipientAddr)
	if err != nil {
		return nil, fmt.Errorf("account address from bech32: %v", err)
	}
	if !transferAmount.IsZero() {
		// Send the transfer amount to the recipient address
		if err := k.bankkeeper.SendCoinsFromModuleToAccount(
			ctx,
			types.ModuleName,
			recipientSDKAddr,
			sdk.NewCoins(sdk.NewCoin(types.WEI, transferAmount)),
		); err != nil {
			return nil, fmt.Errorf("failed to send ETH deposit coins from rollup module to user account %v: %v", recipientSDKAddr, err)
		}
	}

	mintSDKAddr, err := sdk.AccAddressFromBech32(mintAddr)
	if err != nil {
		return nil, fmt.Errorf("account address from bech32: %v", err)
	}
	remainingCoins := mintAmount.Sub(transferAmount)
	if remainingCoins.IsPositive() {
		// Send the remaining mint amount to the deposit tx sender address
		if err := k.bankkeeper.SendCoinsFromModuleToAccount(
			ctx,
			types.ModuleName,
			mintSDKAddr,
			sdk.NewCoins(sdk.NewCoin(types.WEI, remainingCoins)),
		); err != nil {
			return nil, fmt.Errorf("failed to send ETH deposit coins from rollup module to user account %v: %v", mintAddr, err)
		}
	}

	mintEvent := sdk.NewEvent(
		types.EventTypeMintETH,
		sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
		sdk.NewAttribute(types.AttributeKeyMintCosmosAddress, mintAddr),
		sdk.NewAttribute(types.AttributeKeyMint, hexutil.EncodeBig(remainingCoins.BigInt())),
		sdk.NewAttribute(types.AttributeKeyToCosmosAddress, recipientAddr),
		sdk.NewAttribute(types.AttributeKeyValue, hexutil.EncodeBig(transferAmount.BigInt())),
	)

	return &mintEvent, nil
}

// mintERC20 mints a bridged ERC-20 token to an account and returns the associated event.
func (k *Keeper) mintERC20(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	userAddr string,
	erc20addr string,
	amount sdkmath.Int,
) (*sdk.Event, error) {
	// use the "erc20/{l1erc20addr}" format for the coin denom
	coin := sdk.NewCoin("erc20/"+erc20addr[2:], amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to mint ERC-20 deposit coins to the rollup module: %v", err)
	}
	userSDKAddr, err := sdk.AccAddressFromBech32(userAddr)
	if err != nil {
		return nil, fmt.Errorf("account address from bech32: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, userSDKAddr, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to send ERC-20 deposit coins from rollup module to user account %v: %v", userAddr, err)
	}

	mintEvent := sdk.NewEvent(
		types.EventTypeMintERC20,
		sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
		sdk.NewAttribute(types.AttributeKeyToCosmosAddress, userAddr),
		sdk.NewAttribute(types.AttributeKeyERC20Address, erc20addr),
		sdk.NewAttribute(types.AttributeKeyValue, hexutil.EncodeBig(amount.BigInt())),
	)

	return &mintEvent, nil
}

// unpackInputsIntoInterface unpacks the input data of a function call into an interface. This function behaves
// similarly to the geth abi UnpackIntoInterface function but unpacks method inputs instead of outputs.
func unpackInputsIntoInterface(contractABI *abi.ABI, methodName string, inputData []byte, outputInterface interface{}) error {
	method := contractABI.Methods[methodName]

	// Check if the function selector matches the method ID
	functionSelector := inputData[:4]
	if !bytes.Equal(functionSelector, method.ID) {
		return fmt.Errorf("unexpected function selector: got %x, expected %x", functionSelector, method.ID)
	}

	inputs, err := method.Inputs.Unpack(inputData[4:])
	if err != nil {
		return fmt.Errorf("failed to unpack input data: %v", err)
	}

	outputVal := reflect.ValueOf(outputInterface).Elem()
	for i, input := range inputs {
		field := outputVal.Field(i)
		if field.CanSet() {
			val := reflect.ValueOf(input)
			field.Set(val)
		} else {
			return fmt.Errorf("field %d can not be set for method %v", i, methodName)
		}
	}
	return nil
}
