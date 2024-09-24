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
	"github.com/polymerdao/monomer/utils"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

// setL1BlockInfo sets the L1 block info to the app state
//
// Persisted data conforms to optimism specs on L1 attributes:
// https://github.com/ethereum-optimism/optimism/blob/develop/specs/deposits.md#l1-attributes-predeployed-contract
func (k *Keeper) setL1BlockInfo(ctx sdk.Context, info types.L1BlockInfo) error { //nolint:gocritic
	infoBytes, err := info.Marshal()
	if err != nil {
		return types.WrapError(err, "marshal L1 block info")
	}
	if err = k.storeService.OpenKVStore(ctx).Set([]byte(types.KeyL1BlockInfo), infoBytes); err != nil {
		return types.WrapError(err, "set latest L1 block info")
	}
	return nil
}

// processL1AttributesTx processes the L1 Attributes tx and returns the L1 block info.
func (k *Keeper) processL1AttributesTx(ctx sdk.Context, txBytes []byte) (*types.L1BlockInfo, error) { //nolint:gocritic // hugeParam
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
		from, err := ethtypes.MakeSigner(
			monomer.NewChainConfig(tx.ChainId()),
			new(big.Int).SetUint64(l1blockInfo.Number),
			l1blockInfo.Time,
		).Sender(&tx)
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

		// TODO: remove hardcoded address once a genesis state is configured
		// Convert the L1CrossDomainMessenger address to its L2 aliased address
		aliasedL1CrossDomainMessengerAddress := crossdomain.ApplyL1ToL2Alias(common.HexToAddress("0x9A9f2CCfdE556A7E9Ff0848998Aa4a0CFD8863AE"))

		// Check if the tx is a cross domain message from the aliased L1CrossDomainMessenger address
		if from == aliasedL1CrossDomainMessengerAddress && tx.Data() != nil {
			erc20mintEvent, err := k.parseAndExecuteCrossDomainMessage(ctx, tx.Data())
			// TODO: Investigate when to return an error if a cross domain message can't be parsed or executed - look at OP Spec
			if err != nil {
				ctx.Logger().Error("Failed to parse or execute cross domain message", "err", err)
				return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to parse or execute cross domain message: %v", err)
			} else {
				mintEvents = append(mintEvents, *erc20mintEvent)
			}
		}
	}

	return mintEvents, nil
}

// parseAndExecuteCrossDomainMessage parses the tx data of a cross domain message and applies state transitions for recognized messages.
// Currently, only finalizeBridgeERC20 messages from the L1StandardBridge are recognized for minting ERC-20 tokens on the Cosmos chain.
// If a message is not recognized, it returns nil and does not error.
func (k *Keeper) parseAndExecuteCrossDomainMessage(ctx sdk.Context, txData []byte) (*sdk.Event, error) { //nolint:gocritic // hugeParam
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

	// Check if the relayed message is a finalizeBridgeERC20 message from the L1StandardBridge
	if bytes.Equal(relayMessage.Message[:4], standardBridgeABI.Methods["finalizeBridgeERC20"].ID) {
		var finalizeBridgeERC20 bindings.FinalizeBridgeERC20Args
		if err = unpackInputsIntoInterface(
			&standardBridgeABI,
			"finalizeBridgeERC20",
			relayMessage.Message,
			&finalizeBridgeERC20,
		); err != nil {
			return nil, fmt.Errorf("failed to unpack relay message into finalizeBridgeERC20 interface: %v", err)
		}

		// Mint the ERC-20 token to the specified Cosmos address
		mintEvent, err := k.mintERC20(
			ctx,
			utils.EvmToCosmosAddress(finalizeBridgeERC20.To),
			finalizeBridgeERC20.RemoteToken.String(),
			sdkmath.NewIntFromBigInt(finalizeBridgeERC20.Amount),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to mint ERC-20 token: %v", err)
		}

		return mintEvent, nil
	}

	return nil, fmt.Errorf("tx data not recognized as a cross domain message: %v", txData)
}

// mintETH mints ETH to an account where the amount is in wei and returns the associated event.
func (k *Keeper) mintETH(ctx sdk.Context, addr sdk.AccAddress, amount sdkmath.Int) (*sdk.Event, error) { //nolint:gocritic // hugeParam
	coin := sdk.NewCoin(types.ETH, amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to mint ETH deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to send ETH deposit coins from rollup module to user account %v: %v", addr, err)
	}

	mintEvent := sdk.NewEvent(
		types.EventTypeMintETH,
		sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
		sdk.NewAttribute(types.AttributeKeyToCosmosAddress, addr.String()),
		sdk.NewAttribute(types.AttributeKeyValue, hexutil.Encode(amount.BigInt().Bytes())),
	)

	return &mintEvent, nil
}

// mintERC20 mints a bridged ERC-20 token to an account and returns the associated event.
func (k *Keeper) mintERC20(
	ctx sdk.Context, //nolint:gocritic // hugeParam
	userAddr sdk.AccAddress,
	erc20addr string,
	amount sdkmath.Int,
) (*sdk.Event, error) {
	// use the "erc20/{l1erc20addr}" format for the coin denom
	coin := sdk.NewCoin("erc20/"+erc20addr[2:], amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to mint ERC-20 deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, userAddr, sdk.NewCoins(coin)); err != nil {
		return nil, fmt.Errorf("failed to send ERC-20 deposit coins from rollup module to user account %v: %v", userAddr, err)
	}

	mintEvent := sdk.NewEvent(
		types.EventTypeMintERC20,
		sdk.NewAttribute(types.AttributeKeyL1DepositTxType, types.L1UserDepositTxType),
		sdk.NewAttribute(types.AttributeKeyToCosmosAddress, userAddr.String()),
		sdk.NewAttribute(types.AttributeKeyERC20Address, erc20addr),
		sdk.NewAttribute(types.AttributeKeyValue, hexutil.Encode(amount.BigInt().Bytes())),
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
