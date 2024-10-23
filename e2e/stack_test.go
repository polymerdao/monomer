package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/math"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/config"
	cometcore "github.com/cometbft/cometbft/rpc/core/types"
	bfttypes "github.com/cometbft/cometbft/types"
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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
	"github.com/polymerdao/monomer/utils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

const (
	artifactsDirectoryName = "artifacts"
)

func openLogFile(t *testing.T, env *environment.Env, name string) *os.File {
	filename := filepath.Join(artifactsDirectoryName, name+".log")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	env.DeferErr("close log file: "+filename, file.Close)
	return file
}

var e2eTests = []struct {
	name string
	run  func(t *testing.T, stack *e2e.StackConfig)
}{
	{
		name: "ETH L1 Deposits and L2 Withdrawals",
		run:  ethRollupFlow,
	},
	{
		name: "ERC-20 L1 Deposits",
		run:  erc20RollupFlow,
	},
	{
		name: "CometBFT Txs",
		run:  cometBFTtx,
	},
	{
		name: "AttributesTX",
		run:  containsAttributesTx,
	},
	{
		name: "No Rollbacks",
		run:  checkForRollbacks,
	},
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()

	if err := os.Mkdir(artifactsDirectoryName, 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}

	// Unfortunately, geth and parts of the OP Stack occasionally use the root logger.
	// We capture the root logger's output in a separate file.
	log.SetDefault(log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "root-logger"), false)))

	opLogger := log.NewTerminalHandler(openLogFile(t, env, "op"), false)

	prometheusCfg := &config.InstrumentationConfig{
		Prometheus: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stack, err := e2e.Setup(ctx, env, prometheusCfg, &e2e.SelectiveListener{
		OPLogCb: func(r slog.Record) {
			require.NoError(t, opLogger.Handle(context.Background(), r))
		},
		NodeSelectiveListener: &node.SelectiveListener{
			OnEngineHTTPServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnCometServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnPrometheusServeErrCb: func(err error) {
				require.NoError(t, err)
			},
		},
	})
	require.NoError(t, err)

	// Run tests concurrently, against the same stack.
	runningTests := sync.WaitGroup{}
	runningTests.Add(len(e2eTests))

	for _, test := range e2eTests {
		t.Run(test.name, func(t *testing.T) {
			go func() {
				defer runningTests.Done()
				test.run(t, stack)
			}()
		})
	}

	runningTests.Wait()
}

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

func cometBFTtx(t *testing.T, stack *e2e.StackConfig) {
	txBytes := testapp.ToTestTx(t, "userTxKey", "userTxValue")
	bftTx := bfttypes.Tx(txBytes)

	putTx, err := stack.L2Client.BroadcastTxAsync(stack.Ctx, txBytes)
	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, putTx.Code, "put.Code is not OK")
	require.EqualValues(t, bftTx.Hash(), putTx.Hash, "put.Hash does not match local hash")
	t.Log("Monomer can ingest cometbft txs")

	badPutTx := []byte("malformed")
	badPut, err := stack.L2Client.BroadcastTxAsync(stack.Ctx, badPutTx)
	require.NoError(t, err) // no API error - failure encoded in response
	require.NotEqual(t, badPut.Code, abcitypes.CodeTypeOK, "badPut.Code is OK")
	t.Log("Monomer can reject malformed cometbft txs")

	// wait for tx to be processed
	err = stack.WaitL2(1)
	require.NoError(t, err)

	getTx, err := stack.L2Client.Tx(stack.Ctx, bftTx.Hash(), false)

	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, getTx.TxResult.Code, "txResult.Code is not OK")
	require.Equal(t, bftTx, getTx.Tx, "txBytes do not match")
	t.Log("Monomer can serve txs by hash")

	txBlock, err := stack.MonomerClient.BlockByNumber(stack.Ctx, big.NewInt(getTx.Height))
	require.NoError(t, err)
	require.Len(t, txBlock.Transactions(), 2) // 1 deposit tx + 1 cometbft tx
}

func ethRollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client
	monomerClient := stack.MonomerClient

	b, err := monomerClient.BlockByNumber(stack.Ctx, nil)
	require.NoError(t, err, "monomer block by number")
	l2blockGasLimit := b.GasLimit()

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	userPrivKey := stack.Users[0]
	userAddress := crypto.PubkeyToAddress(userPrivKey.PublicKey)
	l1signer := types.NewEIP155Signer(l1ChainID)

	l2GasLimit := l2blockGasLimit / 10
	l1GasLimit := l2GasLimit * 2 // must be higher than l2Gaslimit, because of l1 gas burn (cross-chain gas accounting)

	//////////////////////////
	////// ETH DEPOSITS //////
	//////////////////////////

	// get the user's balance before the deposit has been processed
	balanceBeforeDeposit, err := l1Client.BalanceAt(stack.Ctx, userAddress, nil)
	require.NoError(t, err)

	// send user Deposit Tx
	depositAmount := big.NewInt(params.Ether)
	depositTx, err := stack.OptimismPortal.DepositTransaction(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, depositAmount),
		userAddress,
		depositAmount,
		l2GasLimit,
		false,    // _isCreation
		[]byte{}, // no data
	)
	require.NoError(t, err, "deposit tx")

	// wait for tx to be processed
	// 1 L1 block to process the tx on L1 +
	// 1 L2 block to process the tx on L2
	require.NoError(t, stack.WaitL1(1))
	require.NoError(t, stack.WaitL2(1))

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
		[]common.Address{userAddress},
		[]common.Address{userAddress},
		[]*big.Int{big.NewInt(0)},
	)
	require.NoError(t, err, "configuring 'TransactionDeposited' event listener")
	require.True(t, depositLogs.Next(), "finding deposit event")
	require.NoError(t, depositLogs.Close())

	// get the user's balance after the deposit has been processed
	balanceAfterDeposit, err := stack.L1Client.BalanceAt(stack.Ctx, userAddress, nil)
	require.NoError(t, err)

	//nolint:gocritic
	// gasCost = gasUsed * gasPrice
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), depositTx.GasPrice())

	//nolint:gocritic
	// expectedBalance = balanceBeforeDeposit - depositAmount - gasCost
	expectedBalance := new(big.Int).Sub(new(big.Int).Sub(balanceBeforeDeposit, depositAmount), gasCost)

	// check that the user's balance has been updated on L1
	require.Equal(t, expectedBalance, balanceAfterDeposit)

	userCosmosAddr, err := utils.EvmToCosmosAddress("e2e", userAddress)
	require.NoError(t, err)
	depositValueHex := hexutil.Encode(depositAmount.Bytes())
	requireEthIsMinted(t, stack, userCosmosAddr, depositValueHex)

	t.Log("Monomer can ingest user deposit txs from L1 and mint ETH on L2")

	/////////////////////////////
	////// ETH WITHDRAWALS //////
	/////////////////////////////

	// create a withdrawal tx to withdraw the deposited amount from L2 back to L1
	withdrawalTx := e2e.NewWithdrawalTx(0, userAddress, userAddress, depositAmount, new(big.Int).SetUint64(params.TxGas))

	// initiate the withdrawal of the deposited amount on L2
	senderAddr, err := utils.EvmToCosmosAddress("e2e", *withdrawalTx.Sender)
	require.NoError(t, err)
	withdrawalTxResult, err := stack.L2Client.BroadcastTxAsync(
		stack.Ctx,
		testapp.ToTx(t, &rolluptypes.MsgInitiateWithdrawal{
			Sender:   senderAddr,
			Target:   withdrawalTx.Target.String(),
			Value:    math.NewIntFromBigInt(withdrawalTx.Value),
			GasLimit: withdrawalTx.GasLimit.Bytes(),
			Data:     []byte{},
		}),
	)
	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, withdrawalTxResult.Code)

	// wait for tx to be processed on L2
	require.NoError(t, stack.WaitL2(1))

	// inspect L2 events to ensure that the user's ETH was burned on L2
	requireEthIsBurned(t, stack, userCosmosAddr, depositValueHex)

	// wait for the L2 output containing the withdrawal tx to be proposed on L1
	l2OutputBlockNumber := waitForL2OutputProposal(t, stack.L2OutputOracleCaller)

	// generate the proofs necessary to prove the withdrawal on L1
	provenWithdrawalParams, err := e2e.ProveWithdrawalParameters(stack, *withdrawalTx, l2OutputBlockNumber)
	require.NoError(t, err)

	// send a withdrawal proving tx to prove the withdrawal on L1
	proveWithdrawalTx, err := stack.OptimismPortal.ProveWithdrawalTransaction(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, nil),
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
	finalizationPeriod, err := stack.L2OutputOracleCaller.FinalizationPeriodSeconds(&bind.CallOpts{})
	require.NoError(t, err)
	time.Sleep(time.Duration(finalizationPeriod.Uint64()) * time.Second)

	// get the user's balance before the withdrawal has been finalized
	balanceBeforeFinalization, err := stack.L1Client.BalanceAt(stack.Ctx, userAddress, nil)
	require.NoError(t, err)

	// send a withdrawal finalizing tx to finalize the withdrawal on L1
	finalizeWithdrawalTx, err := stack.OptimismPortal.FinalizeWithdrawalTransaction(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, nil),
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

	// get the user's balance after the withdrawal has been finalized
	balanceAfterFinalization, err := stack.L1Client.BalanceAt(stack.Ctx, userAddress, nil)
	require.NoError(t, err)

	//nolint:gocritic
	// gasCost = gasUsed * gasPrice
	gasCost = new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), finalizeWithdrawalTx.GasPrice())

	//nolint:gocritic
	// expectedBalance = balanceBeforeFinalization + depositAmount - gasCost
	expectedBalance = new(big.Int).Sub(new(big.Int).Add(balanceBeforeFinalization, depositAmount), gasCost)

	// check that the user's balance has been updated on L1
	require.Equal(t, expectedBalance, balanceAfterFinalization)

	t.Log("Monomer can initiate withdrawals on L2 and can generate proofs for verifying the withdrawal on L1")
}

func erc20RollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client
	monomerClient := stack.MonomerClient

	b, err := monomerClient.BlockByNumber(stack.Ctx, nil)
	require.NoError(t, err, "monomer block by number")
	l2blockGasLimit := b.GasLimit()

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	userPrivKey := stack.Users[1]
	userAddress := crypto.PubkeyToAddress(userPrivKey.PublicKey)
	l1signer := types.NewEIP155Signer(l1ChainID)

	l2GasLimit := l2blockGasLimit / 10
	l1GasLimit := l2GasLimit * 2 // must be higher than l2Gaslimit, because of l1 gas burn (cross-chain gas accounting)

	/////////////////////////////
	////// ERC-20 DEPOSITS //////
	/////////////////////////////

	// deploy the WETH9 ERC-20 contract on L1
	weth9Address, tx, WETH9, err := opbindings.DeployWETH9(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, nil),
		l1Client,
	)
	require.NoError(t, err)
	// TODO: we should use wait.ForReceiptOK for all L1 tx receipts in the e2e tests
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err, "waiting for deposit tx on L1")

	// mint some WETH to the user
	wethL1Amount := big.NewInt(params.Ether)
	tx, err = WETH9.Deposit(createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, wethL1Amount))
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)
	wethBalance, err := WETH9.BalanceOf(&bind.CallOpts{}, userAddress)
	require.NoError(t, err)
	require.Equal(t, wethL1Amount, wethBalance)

	// approve WETH9 transfer with the L1StandardBridge address
	tx, err = WETH9.Approve(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, nil),
		stack.L1Deployments.L1StandardBridgeProxy,
		wethL1Amount,
	)
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)

	// bridge the WETH9
	wethL2Amount := big.NewInt(100)
	tx, err = stack.L1StandardBridge.BridgeERC20(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, l1GasLimit, nil),
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
	userAddr, err := utils.EvmToCosmosAddress("e2e", userAddress)
	require.NoError(t, err)
	requireERC20IsMinted(t, stack, userAddr, weth9Address.String(), hexutil.Encode(wethL2Amount.Bytes()))

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
	l1GasLimit uint64,
	value *big.Int,
) *bind.TransactOpts {
	address := crypto.PubkeyToAddress(user.PublicKey)
	return &bind.TransactOpts{
		From: address,
		Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
			signed, err := types.SignTx(tx, l1signer, user)
			require.NoError(t, err)
			return signed, nil
		},
		Nonce:    getCurrentUserNonce(t, stack, address),
		GasPrice: getSuggestedL1GasPrice(t, stack),
		GasLimit: l1GasLimit,
		Value:    value,
		Context:  stack.Ctx,
		NoSend:   false,
	}
}

func getCurrentUserNonce(t *testing.T, stack *e2e.StackConfig, userAddress common.Address) *big.Int {
	nonce, err := stack.L1Client.PendingNonceAt(stack.Ctx, userAddress)
	require.NoError(t, err)
	return new(big.Int).SetUint64(nonce)
}

func getSuggestedL1GasPrice(t *testing.T, stack *e2e.StackConfig) *big.Int {
	gasPrice, err := stack.L1Client.Client.SuggestGasPrice(stack.Ctx)
	require.NoError(t, err)
	return gasPrice
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
