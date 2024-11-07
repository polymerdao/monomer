package e2e_test

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"time"

	authv1beta1 "cosmossdk.io/api/cosmos/auth/v1beta1"
	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	"cosmossdk.io/math"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	cometcore "github.com/cometbft/cometbft/rpc/core/types"
	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cosmossecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/receipts"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/wait"
	"github.com/ethereum-optimism/optimism/op-node/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func checkForRollbacks(t *testing.T, stack *e2e.StackConfig) {
	// Subscribe to new block events
	const subscriber = "rollbackChecker"
	eventChan, err := stack.L2Client.Subscribe(stack.Ctx, subscriber, bfttypes.QueryForEvent(bfttypes.EventNewBlock).String())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, stack.L2Client.UnsubscribeAll(stack.Ctx, subscriber))
	}()

	var lastBlockHeight int64

	// Check new block events for rollbacks
	for event := range eventChan {
		eventNewBlock, ok := event.Data.(bfttypes.EventDataNewBlock)
		require.True(t, ok)
		currentHeight := eventNewBlock.Block.Header.Height

		// Skip the rollback check if lastBlockHeight is not initialized yet
		if lastBlockHeight > 0 {
			// Ensure that the current block height is the last checked height + 1
			require.Equal(t, currentHeight, lastBlockHeight+1, "monomer has rolled back")
		}
		lastBlockHeight = currentHeight

		// Get the L1 block info from the first tx in the block
		ethTxs, err := monomer.AdaptCosmosTxsToEthTxs(eventNewBlock.Block.Txs)
		require.NoError(t, err)
		l1BlockInfo, err := derive.L1BlockInfoFromBytes(&rollup.Config{}, uint64(eventNewBlock.Block.Time.Unix()), ethTxs[0].Data())
		require.NoError(t, err)

		// End the test once a sequencing window has passed.
		if l1BlockInfo.Number >= stack.RollupConfig.SeqWindowSize+1 {
			t.Log("No Monomer rollbacks detected")
			return
		}
	}

	require.Fail(t, "event chan closed prematurely")
}

func containsAttributesTx(t *testing.T, stack *e2e.StackConfig) {
	targetHeight := uint64(5)

	// wait for some blocks to be processed
	err := stack.WaitL2(int(targetHeight))
	require.NoError(t, err)

	for i := uint64(2); i < targetHeight; i++ {
		block, err := stack.MonomerClient.BlockByNumber(stack.Ctx, new(big.Int).SetUint64(i))
		require.NoError(t, err)
		txs := block.Transactions()
		require.GreaterOrEqual(t, len(txs), 1, "expected at least 1 tx in block")
		if tx := txs[0]; !tx.IsDepositTx() {
			txBytes, err := tx.MarshalJSON()
			require.NoError(t, err)
			require.Fail(t, fmt.Sprintf("expected tx to be deposit tx: %s", txBytes))
		}
	}
	t.Log("Monomer blocks contain the l1 attributes deposit tx")
}

func ethRollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	userPrivKey := stack.Users[0]
	userETHAddress := ethcrypto.PubkeyToAddress(userPrivKey.PublicKey)
	l1signer := types.NewLondonSigner(l1ChainID)

	userCosmosETHAddress := monomer.PubkeyToCosmosETHAddress(&userPrivKey.PublicKey)

	//////////////////////////
	////// ETH DEPOSITS //////
	//////////////////////////

	// get the user's balance before the deposit has been processed
	balanceBeforeDeposit, err := l1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	// wait for two blocks
	require.NoError(t, stack.WaitL1(2))

	// send user Deposit Tx
	depositAmount := big.NewInt(params.Ether)
	// https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/tx_helper.go#L38
	depositTx, err := PadGasEstimate(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, depositAmount),
		1.1,
		func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return stack.OptimismPortal.DepositTransaction(
				opts,
				common.Address(userCosmosETHAddress),
				depositAmount,
				100_000,  // l2GasLimit,
				false,    // _isCreation
				[]byte{}, // no data
			)
		},
	)
	require.NoError(t, err)

	// wait for tx to be processed
	// 1 L1 block to process the tx on L1 +
	// 1 L2 block to process the deposit on L2 +
	// 1 L1 block for good measure
	require.NoError(t, stack.WaitL1(1))
	require.NoError(t, stack.WaitL2(1))
	require.NoError(t, stack.WaitL1(1))

	// inspect L1 for deposit tx receipt and emitted TransactionDeposited event
	receipt, err := l1Client.Client.TransactionReceipt(stack.Ctx, depositTx.Hash())
	require.NoError(t, err, "deposit tx receipt")
	require.NotNil(t, receipt, "deposit tx receipt")
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status, "deposit tx reverted")

	depositLogs, err := stack.OptimismPortal.FilterTransactionDeposited(
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[]common.Address{userETHAddress},
		[]common.Address{common.Address(userCosmosETHAddress)},
		[]*big.Int{big.NewInt(0)},
	)
	require.NoError(t, err, "configuring 'TransactionDeposited' event listener")
	require.True(t, depositLogs.Next(), "finding deposit event")
	require.NoError(t, depositLogs.Close())

	// get the user's balance after the deposit has been processed
	balanceAfterDeposit, err := stack.L1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	//nolint:gocritic
	// gasCost = gasUsed * gasPrice
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)

	//nolint:gocritic
	// expectedBalance = balanceBeforeDeposit - depositAmount - gasCost
	expectedBalance := new(big.Int).Sub(new(big.Int).Sub(balanceBeforeDeposit, depositAmount), gasCost)

	// check that the user's balance has been updated on L1
	require.Equal(t, expectedBalance, balanceAfterDeposit)

	userCosmosAddr, err := userCosmosETHAddress.Encode("e2e")
	require.NoError(t, err)
	requireEthIsMinted(t, stack, userCosmosAddr, hexutil.EncodeBig(depositAmount))

	t.Log("Monomer can ingest user deposit txs from L1 and mint ETH on L2")

	/////////////////////////////
	////// ETH WITHDRAWALS //////
	/////////////////////////////

	withdrawalAmount := big.NewInt(1)

	// create a withdrawal tx to withdraw the deposited amount from L2 back to L1
	withdrawalTx := e2e.NewWithdrawalTx(0, common.Address(userCosmosETHAddress), userETHAddress, withdrawalAmount, new(big.Int).SetUint64(params.TxGas))

	queryAccountBytes, err := protov1.Marshal(&authv1beta1.QueryAccountRequest{
		Address: userCosmosAddr,
	})
	require.NoError(t, err)
	queryResult, err := stack.L2Client.ABCIQuery(stack.Ctx, authv1beta1.Query_Account_FullMethodName, queryAccountBytes)
	require.NoError(t, err)
	require.Zero(t, queryResult.Response.Code)
	var accountResponse authv1beta1.QueryAccountResponse
	require.NoError(t, protov1.Unmarshal(queryResult.Response.Value, &accountResponse))
	var baseAccount authv1beta1.BaseAccount
	require.NoError(t, protov1.Unmarshal(accountResponse.Account.Value, &baseAccount))

	l2ChainID, err := stack.MonomerClient.ChainID(stack.Ctx)
	require.NoError(t, err)

	withdrawalTxResult, err := stack.L2Client.BroadcastTxAsync(
		stack.Ctx,
		buildTx(t, l2ChainID.String(), baseAccount.Sequence, baseAccount.AccountNumber, userPrivKey, []protov1.Message{
			&rolluptypes.MsgInitiateWithdrawal{
				Sender:   userCosmosAddr,
				Target:   withdrawalTx.Target.String(),
				Value:    math.NewIntFromBigInt(withdrawalTx.Value),
				GasLimit: withdrawalTx.GasLimit.Bytes(),
				Data:     []byte{},
			},
		}),
	)
	require.NoError(t, err)
	require.Equalf(t, abcitypes.CodeTypeOK, withdrawalTxResult.Code, "log: "+withdrawalTxResult.Log)

	// wait for tx to be processed on L2
	require.NoError(t, stack.WaitL2(2))

	// inspect L2 events to ensure that the user's ETH was burned on L2
	requireEthIsBurned(t, stack, userCosmosAddr, hexutil.EncodeBig(withdrawalAmount))

	// wait for the L2 output containing the withdrawal tx to be proposed on L1
	var l2OutputBlockNumber *big.Int
	for range 3 { // A bit hacky to just wait for 3 outputs, but should be sufficient.
		l2OutputBlockNumber = waitForL2OutputProposal(t, stack.L2OutputOracleCaller)
	}

	// generate the proofs necessary to prove the withdrawal on L1
	provenWithdrawalParams, err := e2e.ProveWithdrawalParameters(stack, *withdrawalTx, l2OutputBlockNumber)
	require.NoError(t, err)

	// send a withdrawal proving tx to prove the withdrawal on L1
	proveWithdrawalTx, err := stack.OptimismPortal.ProveWithdrawalTransaction(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		withdrawalTx.WithdrawalTransaction(),
		provenWithdrawalParams.L2OutputIndex,
		provenWithdrawalParams.OutputRootProof,
		provenWithdrawalParams.WithdrawalProof,
	)
	require.NoError(t, err, "prove withdrawal tx")

	// wait for withdrawal proving tx to be processed on L1
	require.NoError(t, stack.WaitL1(1))

	// inspect L1 for withdrawal proving tx receipt and emitted WithdrawalProven event
	receipt, err = l1Client.Client.TransactionReceipt(stack.Ctx, proveWithdrawalTx.Hash())
	require.NoError(t, err, "withdrawal proving tx receipt")
	require.NotNil(t, receipt, "withdrawal proving tx receipt")
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status, "withdrawal proving tx failed")

	withdrawalTxHash, err := withdrawalTx.Hash()
	require.NoError(t, err)
	proveWithdrawalLogs, err := stack.OptimismPortal.FilterWithdrawalProven(
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[][32]byte{[32]byte(withdrawalTxHash.Bytes())},
		[]common.Address{*withdrawalTx.Sender},
		[]common.Address{*withdrawalTx.Target},
	)
	require.NoError(t, err, "configuring 'WithdrawalProven' event listener")
	require.True(t, proveWithdrawalLogs.Next(), "finding WithdrawalProven event")
	require.NoError(t, proveWithdrawalLogs.Close())

	// wait for the withdrawal finalization period before sending the withdrawal finalizing tx
	// TODO why do we need to wait this long? I tried calling OptimismPortal.IsOutputFinalized:
	// even when it returned true, FinalizeWithdrawalTransaction would fail.
	require.NoError(t, stack.WaitL1(2))

	// send a withdrawal finalizing tx to finalize the withdrawal on L1
	finalizeWithdrawalTx, err := stack.OptimismPortal.FinalizeWithdrawalTransaction(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		withdrawalTx.WithdrawalTransaction(),
	)
	require.NoError(t, err)

	// wait for withdrawal finalizing tx to be processed on L1
	require.NoError(t, stack.WaitL1(1))

	// inspect L1 for withdrawal finalizing tx receipt and emitted WithdrawalFinalized event
	receipt, err = l1Client.Client.TransactionReceipt(stack.Ctx, finalizeWithdrawalTx.Hash())
	require.NoError(t, err, "finalize withdrawal tx receipt")
	require.NotNil(t, receipt, "finalize withdrawal tx receipt")
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status, "finalize withdrawal tx failed")

	finalizeWithdrawalLogs, err := stack.OptimismPortal.FilterWithdrawalFinalized(
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[][32]byte{[32]byte(withdrawalTxHash.Bytes())},
	)
	require.NoError(t, err, "configuring 'WithdrawalFinalized' event listener")
	require.True(t, finalizeWithdrawalLogs.Next(), "finding WithdrawalFinalized event")
	require.True(t, finalizeWithdrawalLogs.Event.Success, "withdrawal finalization failed")
	require.NoError(t, finalizeWithdrawalLogs.Close())

	t.Log("Monomer can initiate withdrawals on L2 and can generate proofs for verifying the withdrawal on L1")
}

func convertPrivKey(ecdsaPrivKey *ecdsa.PrivateKey) *secp256k1.PrivateKey {
	privKeyBytes := ecdsaPrivKey.D.Bytes()
	var key secp256k1.ModNScalar
	if len(privKeyBytes) > 32 || key.SetByteSlice(privKeyBytes) {
		panic("overflow")
	}
	if key.IsZero() {
		panic("private keys must not be 0")
	}
	return secp256k1.NewPrivateKey(&key)
}

func buildTx(t *testing.T, chainID string, seqNum, accNum uint64, ethPrivKey *ecdsa.PrivateKey, msgs []protov1.Message) bfttypes.Tx {
	cosmosPrivKey := &cosmossecp256k1.PrivKey{
		Key: convertPrivKey(ethPrivKey).Serialize(),
	}

	var msgAnys []*codectypes.Any
	for _, msg := range msgs {
		msgAny, err := codectypes.NewAnyWithValue(msg)
		require.NoError(t, err)
		msgAnys = append(msgAnys, msgAny)
	}

	pubKeyAny, err := codectypes.NewAnyWithValue(cosmosPrivKey.PubKey())
	require.NoError(t, err)

	tx := &sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: msgAnys,
		},
		AuthInfo: &sdktx.AuthInfo{
			SignerInfos: []*sdktx.SignerInfo{
				{
					PublicKey: pubKeyAny,
					ModeInfo: &sdktx.ModeInfo{
						Sum: &sdktx.ModeInfo_Single_{
							Single: &sdktx.ModeInfo_Single{
								Mode: signing.SignMode_SIGN_MODE_DIRECT,
							},
						},
					},
					Sequence: seqNum,
				},
			},
			Fee: &sdktx.Fee{
				Amount:   sdk.NewCoins(sdk.NewCoin(rolluptypes.WEI, math.NewInt(100000000))),
				GasLimit: 1000000,
			},
		},
	}

	bodyBytes, err := protov1.Marshal(tx.Body)
	require.NoError(t, err)
	authInfoBytes, err := protov1.Marshal(tx.AuthInfo)
	require.NoError(t, err)

	signBytes, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&txv1beta1.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		ChainId:       chainID,
		AccountNumber: accNum,
	})
	require.NoError(t, err)

	signature, err := cosmosPrivKey.Sign(signBytes)
	require.NoError(t, err)

	require.True(t, cosmosPrivKey.PubKey().VerifySignature(signBytes, signature))

	tx.Signatures = [][]byte{signature}

	txBytes, err := tx.Marshal()
	require.NoError(t, err)
	return txBytes
}

func erc20RollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	userPrivKey := stack.Users[1]
	userAddress := ethcrypto.PubkeyToAddress(userPrivKey.PublicKey)
	l1signer := types.NewLondonSigner(l1ChainID)

	/////////////////////////////
	////// ERC-20 DEPOSITS //////
	/////////////////////////////

	// deploy the WETH9 ERC-20 contract on L1
	weth9Address, tx, WETH9, err := opbindings.DeployWETH9(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		l1Client,
	)
	require.NoError(t, err)
	// TODO: we should use wait.ForReceiptOK for all L1 tx receipts in the e2e tests
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err, "waiting for deposit tx on L1")

	// mint some WETH to the user
	wethL1Amount := big.NewInt(params.Ether)
	tx, err = WETH9.Deposit(createL1TransactOpts(t, stack, userPrivKey, l1signer, wethL1Amount))
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)
	wethBalance, err := WETH9.BalanceOf(&bind.CallOpts{}, userAddress)
	require.NoError(t, err)
	require.Equal(t, wethL1Amount, wethBalance)

	// approve WETH9 transfer with the L1StandardBridge address
	tx, err = WETH9.Approve(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		stack.L1Deployments.L1StandardBridgeProxy,
		wethL1Amount,
	)
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)

	// bridge the WETH9
	wethL2Amount := big.NewInt(100)
	tx, err = stack.L1StandardBridge.BridgeERC20(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		weth9Address,
		weth9Address,
		wethL2Amount,
		100_000,
		[]byte{},
	)
	require.NoError(t, err)
	depositReceipt, err := wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)

	// check that the deposit tx went through the OptimismPortal successfully
	_, err = receipts.FindLog(depositReceipt.Logs, stack.OptimismPortal.ParseTransactionDeposited)
	require.NoError(t, err, "should emit deposit event")

	// assert the user's bridged WETH is no longer on L1
	wethBalance, err = WETH9.BalanceOf(&bind.CallOpts{}, userAddress)
	require.NoError(t, err)
	require.Equal(t, new(big.Int).Sub(wethL1Amount, wethL2Amount), wethBalance)

	// wait for tx to be processed
	// 1 L1 block to process the tx on L1 +
	// 1 L2 block to process the tx on L2
	require.NoError(t, stack.WaitL1(1))
	require.NoError(t, stack.WaitL2(1))

	// assert the user's bridged WETH is on L2
	userAddr, err := monomer.CosmosETHAddress(userAddress).Encode("e2e")
	require.NoError(t, err)
	requireERC20IsMinted(t, stack, userAddr, weth9Address.String(), hexutil.EncodeBig(wethL2Amount))

	t.Log("Monomer can ingest ERC-20 deposit txs from L1 and mint ERC-20 tokens on L2")
}

func requireEthIsMinted(t *testing.T, stack *e2e.StackConfig, userAddress, valueHex string) {
	query := fmt.Sprintf(
		"%s.%s='%s' AND %s.%s='%s' AND %s.%s='%s'",
		rolluptypes.EventTypeMintETH, rolluptypes.AttributeKeyL1DepositTxType, rolluptypes.L1UserDepositTxType,
		rolluptypes.EventTypeMintETH, rolluptypes.AttributeKeyToCosmosAddress, userAddress,
		rolluptypes.EventTypeMintETH, rolluptypes.AttributeKeyValue, valueHex,
	)
	result := l2TxSearch(t, stack, query)

	require.NotEmpty(t, result.Txs, "mint_eth event not found")
}

func requireEthIsBurned(t *testing.T, stack *e2e.StackConfig, userAddress, valueHex string) {
	query := fmt.Sprintf(
		"%s.%s='%s' AND %s.%s='%s' AND %s.%s='%s'",
		rolluptypes.EventTypeBurnETH, rolluptypes.AttributeKeyL2WithdrawalTx, rolluptypes.EventTypeWithdrawalInitiated,
		rolluptypes.EventTypeBurnETH, rolluptypes.AttributeKeyFromCosmosAddress, userAddress,
		rolluptypes.EventTypeBurnETH, rolluptypes.AttributeKeyValue, valueHex,
	)
	result := l2TxSearch(t, stack, query)

	require.NotEmpty(t, result.Txs, "burn_eth event not found")
}

func requireERC20IsMinted(t *testing.T, stack *e2e.StackConfig, userAddress, tokenAddress, valueHex string) {
	query := fmt.Sprintf(
		"%s.%s='%s' AND %s.%s='%s' AND %s.%s='%s' AND %s.%s='%s'",
		rolluptypes.EventTypeMintERC20, rolluptypes.AttributeKeyL1DepositTxType, rolluptypes.L1UserDepositTxType,
		rolluptypes.EventTypeMintERC20, rolluptypes.AttributeKeyToCosmosAddress, userAddress,
		rolluptypes.EventTypeMintERC20, rolluptypes.AttributeKeyERC20Address, tokenAddress,
		rolluptypes.EventTypeMintERC20, rolluptypes.AttributeKeyValue, valueHex,
	)
	result := l2TxSearch(t, stack, query)

	require.NotEmpty(t, result.Txs, "mint_erc20 event not found")
}

func l2TxSearch(t *testing.T, stack *e2e.StackConfig, query string) *cometcore.ResultTxSearch {
	page := 1
	perPage := 100

	result, err := stack.L2Client.TxSearch(
		stack.Ctx,
		query,
		false,
		&page,
		&perPage,
		"desc",
	)
	require.NoError(t, err, "search transactions")
	require.NotNil(t, result)
	return result
}

func createL1TransactOpts(
	t *testing.T,
	stack *e2e.StackConfig,
	user *ecdsa.PrivateKey,
	l1signer types.Signer,
	value *big.Int,
) *bind.TransactOpts {
	return &bind.TransactOpts{
		From: ethcrypto.PubkeyToAddress(user.PublicKey),
		Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
			signed, err := types.SignTx(tx, l1signer, user)
			require.NoError(t, err)
			return signed, nil
		},
		Value:   value,
		Context: stack.Ctx,
	}
}

// waitForL2OutputProposal waits for the L2 output containing the withdrawal tx to be proposed on L1 and returns
// the block number with the L2 output proposal.
func waitForL2OutputProposal(t *testing.T, l2OutputOracleCaller *bindings.L2OutputOracleCaller) *big.Int {
	// get the L2 block number where the withdrawal tx will be included in an output proposal
	nextOutputBlockNumber, err := l2OutputOracleCaller.NextBlockNumber(&bind.CallOpts{})
	require.NoError(t, err)

	// wait for the L2 output containing the withdrawal tx to be proposed on L1
	l2OutputBlockNumber, err := l2OutputOracleCaller.LatestBlockNumber(&bind.CallOpts{})
	require.NoError(t, err)
	for l2OutputBlockNumber.Cmp(nextOutputBlockNumber) < 0 {
		l2OutputBlockNumber, err = l2OutputOracleCaller.LatestBlockNumber(&bind.CallOpts{})
		require.NoError(t, err)

		time.Sleep(250 * time.Millisecond)
	}

	return l2OutputBlockNumber
}

// https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/e2eutils/transactions/gas.go#L18
// TxBuilder creates and sends a transaction using the supplied bind.TransactOpts.
// Returns the created transaction and any error reported.
type TxBuilder func(opts *bind.TransactOpts) (*types.Transaction, error)

func PadGasEstimate(opts *bind.TransactOpts, paddingFactor float64, builder TxBuilder) (*types.Transaction, error) {
	// Take a copy of the opts to avoid mutating the original
	oCopy := *opts
	o := &oCopy
	o.NoSend = true
	tx, err := builder(o)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas: %w", err)
	}
	gas := float64(tx.Gas()) * paddingFactor
	o.GasLimit = uint64(gas)
	o.NoSend = false
	return builder(o)
}
