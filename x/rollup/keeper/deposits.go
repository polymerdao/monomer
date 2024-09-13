package keeper

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"strings"

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
		to := tx.To()
		// if the receipient is nil, it means the tx is creating a contract which we don't support, so return an error.
		// see https://github.com/ethereum-optimism/op-geth/blob/v1.101301.0-rc.2/core/state_processor.go#L154
		if to == nil {
			ctx.Logger().Error("Contract creation txs are not supported", "index", i)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "Contract creation txs are not supported, index:%d", i)
		}
		cosmAddr := utils.EvmToCosmosAddress(*to)
		mintAmount := sdkmath.NewIntFromBigInt(tx.Value())
		mintEvent, err := k.mintETH(ctx, cosmAddr, mintAmount)
		if err != nil {
			ctx.Logger().Error("Failed to mint ETH", "evmAddress", to, "cosmosAddress", cosmAddr, "err", err)
			return nil, types.WrapError(types.ErrMintETH, "failed to mint ETH for cosmosAddress: %v; err: %v", cosmAddr, err)
		}
		mintEvents = append(mintEvents, *mintEvent)

		from, err := ethtypes.NewLondonSigner(tx.ChainId()).Sender(&tx)
		if err != nil {
			ctx.Logger().Error("Failed to get sender from tx", "index", i, "err", err)
			return nil, types.WrapError(types.ErrInvalidL1Txs, "failed to get sender from tx, index:%d, err:%v", i, err)
		}

		fmt.Printf("tx sender: %s\n", from.String())

		// TODO: figure out actual addresses
		var (
			//L1StandardBridgeAddress       = common.HexToAddress("0x420")
			L2CrossDomainMessengerAddress = common.HexToAddress("0x4200000000000000000000000000000000000007")
		)

		// TODO: need to figure out what to assert with the `from` address to ensure the tx is coming from the L1StandardBridge? Also, do we need to assert tx.SourceHash?
		// TODO: add better check for whether deposit tx data is ETH or ERC-20. Investigate how security would work, ensure users can't forge tx data
		if *to == L2CrossDomainMessengerAddress && tx.Data() != nil {

			// TODO: remove all temp testing code
			// TODO: pull ABI from bindings instead of copying it here?
			// ABI definitions for both functions
			const relayMessageABI = `[{"inputs":[{"internalType":"uint256","name":"_nonce","type":"uint256"},{"internalType":"address","name":"_sender","type":"address"},{"internalType":"address","name":"_target","type":"address"},{"internalType":"uint256","name":"_value","type":"uint256"},{"internalType":"uint32","name":"_minGasLimit","type":"uint32"},{"internalType":"bytes","name":"_message","type":"bytes"}],"name":"relayMessage","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
			const finalizeBridgeERC20ABI = `[{"inputs":[{"internalType":"address","name":"_remoteToken","type":"address"},{"internalType":"address","name":"_localToken","type":"address"},{"internalType":"address","name":"_from","type":"address"},{"internalType":"address","name":"_to","type":"address"},{"internalType":"uint256","name":"_amount","type":"uint256"},{"internalType":"bytes","name":"_extraData","type":"bytes"}],"name":"finalizeBridgeERC20","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

			relayAbi, err := abi.JSON(strings.NewReader(relayMessageABI))
			if err != nil {
				return nil, fmt.Errorf("failed to parse relayMessage ABI: %v", err)
			}
			finalizeAbi, err := abi.JSON(strings.NewReader(finalizeBridgeERC20ABI))
			if err != nil {
				return nil, fmt.Errorf("failed to parse finalizeBridgeERC20 ABI: %v", err)
			}

			// Skip the function selector
			data := tx.Data()[4:]

			relayMessage, err := relayAbi.Methods["relayMessage"].Inputs.Unpack(data)
			if err != nil {
				return nil, fmt.Errorf("failed to unpack outer parameters: %v", err)
			}

			nonce := relayMessage[0].(*big.Int)
			sender := relayMessage[1].(common.Address)
			target := relayMessage[2].(common.Address)
			value := relayMessage[3].(*big.Int)
			minGasLimit := relayMessage[4].(uint32)
			innerMessage := relayMessage[5].([]byte) // finalizeBridgeERC20

			fmt.Printf("Decoded Relay Message\n")
			fmt.Printf("Nonce: %s\n", nonce.String())
			fmt.Printf("Sender: %s\n", sender.Hex())
			fmt.Printf("Target: %s\n", target.Hex())
			fmt.Printf("Value: %s\n", value.String())
			fmt.Printf("Min Gas Limit: %d\n\n", minGasLimit)

			decodedFinalizeBridgeERC20, err := finalizeAbi.Methods["finalizeBridgeERC20"].Inputs.Unpack(innerMessage[4:]) // Skip the selector
			if err != nil {
				return nil, fmt.Errorf("failed to unpack inner parameters: %v", err)
			}

			remoteToken := decodedFinalizeBridgeERC20[0].(common.Address)
			localToken := decodedFinalizeBridgeERC20[1].(common.Address)
			erc20From := decodedFinalizeBridgeERC20[2].(common.Address)
			erc20To := decodedFinalizeBridgeERC20[3].(common.Address)
			amount := decodedFinalizeBridgeERC20[4].(*big.Int)
			extraData := decodedFinalizeBridgeERC20[5].([]byte)

			fmt.Printf("Decoded finalizeBridgeERC20\n")
			fmt.Printf("Remote Token: %s\n", remoteToken.Hex())
			fmt.Printf("Local Token: %s\n", localToken.Hex())
			fmt.Printf("From: %s\n", erc20From.Hex())
			fmt.Printf("To: %s\n", erc20To.Hex())
			fmt.Printf("Amount: %s\n", amount.String())
			fmt.Printf("Extra Data: %x\n", extraData)

			// Mint the ERC20 token
			err = k.mintERC20(ctx, utils.EvmToCosmosAddress(erc20To), remoteToken.String(), sdkmath.NewIntFromBigInt(amount))
		}
	}

	return mintEvents, nil
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
func (k *Keeper) mintERC20(ctx sdk.Context, userAddr sdk.AccAddress, erc20addr string, amount sdkmath.Int) error { //nolint:gocritic // hugeParam
	// use the "erc20/{erc20addr}" format as the coin denom
	coin := sdk.NewCoin("erc20/"+erc20addr[2:], amount)
	if err := k.bankkeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to mint ERC-20 deposit coins to the rollup module: %v", err)
	}
	if err := k.bankkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, userAddr, sdk.NewCoins(coin)); err != nil {
		return fmt.Errorf("failed to send ERC-20 deposit coins from rollup module to user account %v: %v", userAddr, err)
	}

	fmt.Printf("Minted %s %s to %s\n", coin.Amount, coin.Denom, userAddr.String())

	return nil
}
