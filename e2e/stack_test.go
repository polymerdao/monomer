package e2e_test

import (
	"context"
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
	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	bfttypes "github.com/cometbft/cometbft/types"
	indexerbindings "github.com/ethereum-optimism/optimism/indexer/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
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
	oneEth                 = 1e18
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
		name: "L1 Deposits and L2 Withdrawals",
		run:  rollupFlow,
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

func rollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client
	monomerClient := stack.MonomerClient

	b, err := monomerClient.BlockByNumber(stack.Ctx, nil)
	require.NoError(t, err, "monomer block by number")
	l2blockGasLimit := b.GasLimit()

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	user := stack.Users[0]
	l1signer := types.NewEIP155Signer(l1ChainID)

	//////////////////////
	////// DEPOSITS //////
	//////////////////////

	// TODO: helper func for getting latest user nonce and suggested gas price?
	nonce, err := l1Client.Client.NonceAt(stack.Ctx, user.Address, nil)
	require.NoError(t, err)

	gasPrice, err := l1Client.Client.SuggestGasPrice(stack.Ctx)
	require.NoError(t, err)

	l2GasLimit := l2blockGasLimit / 10
	//l1GasLimit := l2GasLimit * 2 // must be higher than l2Gaslimit, because of l1 gas burn (cross-chain gas accounting)
	l1GasLimit := uint64(5_000_000)

	// send user Deposit Tx
	depositAmount := big.NewInt(oneEth / 2)
	depositTx, err := stack.L1Portal.DepositTransaction(
		&bind.TransactOpts{
			From: user.Address,
			Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
				signed, err := types.SignTx(tx, l1signer, user.PrivateKey)
				if err != nil {
					return nil, err
				}
				return signed, nil
			},
			Nonce:    big.NewInt(int64(nonce)),
			GasPrice: big.NewInt(gasPrice.Int64() * 2),
			GasLimit: l1GasLimit,
			Value:    big.NewInt(oneEth),
			Context:  stack.Ctx,
			NoSend:   false,
		},
		user.Address,
		depositAmount, // the "minting order" for L2
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

	depositLogs, err := stack.L1Portal.FilterTransactionDeposited(
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[]common.Address{user.Address},
		[]common.Address{user.Address},
		[]*big.Int{big.NewInt(0)},
	)
	require.NoError(t, err, "configuring 'TransactionDeposited' event listener")
	if !depositLogs.Next() {
		require.FailNowf(t, "finding deposit event", "err: %v", depositLogs.Error())
	}
	t.Log("Monomer can send user deposit txs on L1")

	err = depositLogs.Close()
	require.NoError(t, err)

	requireEthIsMinted(t, stack.L2Client)

	/////////////////////////
	////// WITHDRAWALS //////
	/////////////////////////

	withdrawalTx := e2e.NewWithdrawalTx(0, user.Address, user.Address, depositAmount)
	require.NoError(t, err)
	withdrawalTxHash, err := withdrawalTx.Hash()
	require.NoError(t, err)

	// initiate the withdrawal of the deposited amount on L2
	withdrawalTxResult, err := stack.L2Client.BroadcastTxAsync(
		stack.Ctx,
		testapp.ToWithdrawalTx(t, utils.EvmToCosmosAddress(*withdrawalTx.Sender).String(), withdrawalTx.Target.String(), math.NewIntFromBigInt(withdrawalTx.Value)),
	)
	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, withdrawalTxResult.Code)

	// wait for tx to be processed
	require.NoError(t, stack.WaitL2(1))

	// TODO: rename requireEthIsBurned
	l2BlockNumber := requireEthIsBurned(t, stack.L2Client)

	// TODO: should we listen for events here instead?
	// TODO: helper func?
	// wait for the L2 output containing the withdrawal tx to be proposed on L1
	l2OutputBlockNr, err := stack.L2OutputOracleCaller.LatestBlockNumber(&bind.CallOpts{})
	require.NoError(t, err)
	// TODO: use channels instead for a timeout?
	for l2OutputBlockNr.Cmp(l2BlockNumber) < 0 {
		l2OutputBlockNr, err = stack.L2OutputOracleCaller.LatestBlockNumber(&bind.CallOpts{})
		require.NoError(t, err)

		time.Sleep(250 * time.Millisecond)
	}

	nonce, err = l1Client.Client.NonceAt(stack.Ctx, user.Address, nil)
	require.NoError(t, err)

	provenWithdrawalParams, err := e2e.ProveWithdrawalParameters(stack.Ctx, stack, *withdrawalTx, l2OutputBlockNr, stack.L2OutputOracleCaller)
	require.NoError(t, err)
	outputRootProof := provenWithdrawalParams.OutputRootProof

	// TODO: remove temp testing code
	//stateRoot := string(outputRootProof.StateRoot[:])
	//fmt.Printf("stateRoot: %s\n", stateRoot)
	//messagePasserStorageRoot := string(outputRootProof.MessagePasserStorageRoot[:])
	//fmt.Printf("messagePasserStorageRoot: %s\n", messagePasserStorageRoot)
	//blockHash := string(outputRootProof.LatestBlockhash[:])
	//fmt.Printf("blockHash: %s\n", blockHash)

	proveWithdrawalTx, err := stack.L1Portal.ProveWithdrawalTransaction(
		// TODO: make helper func for TransactOpts. maybe include nonce generation and gas price calculation too?
		&bind.TransactOpts{
			From: user.Address,
			Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
				signed, err := types.SignTx(tx, l1signer, user.PrivateKey)
				if err != nil {
					return nil, err
				}
				return signed, nil
			},
			Nonce:    big.NewInt(int64(nonce)),
			GasPrice: big.NewInt(gasPrice.Int64() * 2),
			GasLimit: l1GasLimit,
			Context:  stack.Ctx,
			NoSend:   false,
		},
		// TODO: add helper func for indexerbindings cast since it's used in multiple places
		indexerbindings.TypesWithdrawalTransaction{
			Nonce:    withdrawalTx.Nonce,
			Sender:   *withdrawalTx.Sender,
			Target:   *withdrawalTx.Target,
			Value:    withdrawalTx.Value,
			GasLimit: withdrawalTx.GasLimit,
			Data:     withdrawalTx.Data,
		},
		provenWithdrawalParams.L2OutputIndex,
		indexerbindings.TypesOutputRootProof{
			Version:                  outputRootProof.Version,
			StateRoot:                outputRootProof.StateRoot,
			MessagePasserStorageRoot: outputRootProof.MessagePasserStorageRoot,
			LatestBlockhash:          outputRootProof.LatestBlockhash,
		},
		provenWithdrawalParams.WithdrawalProof,
	)
	require.NoError(t, err, "prove withdrawal tx")

	// wait for withdrawal proving tx to be processed
	require.NoError(t, stack.WaitL1(1))

	// inspect L1 for withdrawal proving tx receipt and emitted WithdrawalProven event
	receipt, err = l1Client.Client.TransactionReceipt(stack.Ctx, proveWithdrawalTx.Hash())
	require.NoError(t, err, "withdrawal proving tx receipt")
	require.NotNil(t, receipt, "withdrawal proving tx receipt")
	// TODO: remove! temp debugging to figure out why contract call is reverting
	//l1portalAddress := common.HexToAddress("0x9A676e781A523b5d0C0e43731313A708CB607508")
	//msg := ethereum.CallMsg{
	//	To:   &l1portalAddress,
	//	Data: proveWithdrawalTx.Data(),
	//}
	//_, err = stack.L1Client.CallContract(stack.Ctx, msg, receipt.BlockNumber)
	//require.NoError(t, err, "call contract")
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status, "withdrawal proving tx failed")

	proveWithdrawalLogs, err := stack.L1Portal.FilterWithdrawalProven(
		// TODO: helper func for FilterOpts? maybe unnecessary since it's small
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[][32]byte{[32]byte(withdrawalTxHash.Bytes())},
		[]common.Address{user.Address},
		[]common.Address{*withdrawalTx.Target},
	)
	require.NoError(t, err, "configuring 'WithdrawalProven' event listener")
	if !proveWithdrawalLogs.Next() {
		require.FailNowf(t, "finding WithdrawalProven event", "err: %v", proveWithdrawalLogs.Error())
	}
	t.Log("Monomer can prove withdrawals on L1")

	err = proveWithdrawalLogs.Close()
	require.NoError(t, err)

	//l1Block, err := stack.L1Client.BlockByNumber(stack.Ctx, receipt.BlockNumber)
	//require.NoError(t, err)
	//fmt.Printf("l1Block time (proving): %v\n", l1Block.Time())
	//finalizationPeriod, err := l2OutputOracleCaller.FinalizationPeriodSeconds(&bind.CallOpts{})
	//fmt.Printf("finalizationPeriod: %v\n", finalizationPeriod)

	// wait for the withdrawal finalization period before sending the withdrawal finalizing tx
	// TODO: maybe call isOutputFinalized func in OPPortal instead to see if output is finalized?
	finalizationPeriod, err := stack.L2OutputOracleCaller.FinalizationPeriodSeconds(&bind.CallOpts{})
	require.NoError(t, err)
	time.Sleep(time.Duration(finalizationPeriod.Uint64()) * time.Second)

	nonce, err = l1Client.Client.NonceAt(stack.Ctx, user.Address, nil)
	require.NoError(t, err)

	finalizeWithdrawalTx, err := stack.L1Portal.FinalizeWithdrawalTransaction(
		&bind.TransactOpts{
			From: user.Address,
			Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
				signed, err := types.SignTx(tx, l1signer, user.PrivateKey)
				if err != nil {
					return nil, err
				}
				return signed, nil
			},
			Nonce:    big.NewInt(int64(nonce)),
			GasPrice: big.NewInt(gasPrice.Int64() * 2),
			GasLimit: l1GasLimit,
			Context:  stack.Ctx,
			NoSend:   false,
		}, indexerbindings.TypesWithdrawalTransaction{
			Nonce:    withdrawalTx.Nonce,
			Sender:   *withdrawalTx.Sender,
			Target:   *withdrawalTx.Target,
			Value:    withdrawalTx.Value,
			GasLimit: withdrawalTx.GasLimit,
			Data:     withdrawalTx.Data,
		})
	require.Nil(t, err)

	// wait for withdrawal finalizing tx to be processed
	require.NoError(t, stack.WaitL1(1))

	// inspect L1 for withdrawal finalizing tx receipt and emitted WithdrawalFinalized event
	receipt, err = l1Client.Client.TransactionReceipt(stack.Ctx, finalizeWithdrawalTx.Hash())
	require.NoError(t, err, "finalize withdrawal tx receipt")
	require.NotNil(t, receipt, "finalize withdrawal tx receipt")

	// TODO: remove! temp debugging to figure out why contract call is reverting
	//l1Block, err = stack.L1Client.BlockByNumber(stack.Ctx, nil)
	//require.NoError(t, err)
	//fmt.Printf("l1Block time (finalizing): %v\n", l1Block.Time())
	//l1portalAddress := common.HexToAddress("0x9A676e781A523b5d0C0e43731313A708CB607508")
	//msg := ethereum.CallMsg{
	//	To:   &l1portalAddress,
	//	Data: finalizeWithdrawalTx.Data(),
	//}
	//callResult, err := stack.L1Client.CallContract(stack.Ctx, msg, receipt.BlockNumber)
	//require.NoError(t, err, "call contract")
	//fmt.Printf("call result: %v\n", callResult)

	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status, "finalize withdrawal tx failed")

	finalizeWithdrawalLogs, err := stack.L1Portal.FilterWithdrawalFinalized(
		// TODO: helper func for FilterOpts? maybe unnecessary since it's small
		&bind.FilterOpts{
			Start:   0,
			End:     nil,
			Context: stack.Ctx,
		},
		[][32]byte{[32]byte(withdrawalTxHash.Bytes())},
	)
	require.NoError(t, err, "configuring 'WithdrawalFinalized' event listener")
	if !finalizeWithdrawalLogs.Next() {
		require.FailNowf(t, "finding WithdrawalFinalized event", "err: %v", finalizeWithdrawalLogs.Error())
	}
	require.True(t, finalizeWithdrawalLogs.Event.Success, "withdrawal finalization failed")
	t.Log("Monomer can finalize withdrawals on L1")

	err = finalizeWithdrawalLogs.Close()
	require.NoError(t, err)
}

func requireEthIsMinted(t *testing.T, appchainClient *bftclient.HTTP) {
	query := fmt.Sprintf(
		"%s.%s='%s'",
		rolluptypes.EventTypeMintETH,
		rolluptypes.AttributeKeyL1DepositTxType,
		rolluptypes.L1UserDepositTxType,
	)
	page := 1
	perPage := 100
	orderBy := "desc"

	result, err := appchainClient.TxSearch(
		context.Background(),
		query,
		false,
		&page,
		&perPage,
		orderBy,
	)
	require.NoError(t, err, "search transactions")
	require.NotNil(t, result)
	require.NotEmpty(t, result.Txs, "mint_eth event not found")
	t.Log("Monomer can mint ETH for L1 user deposits")
}

// TODO: refactor shared code with requireEthIsMinted
// TODO: rename func with returning block height?
func requireEthIsBurned(t *testing.T, appchainClient *bftclient.HTTP) *big.Int {
	query := fmt.Sprintf(
		"%s.%s='%s'",
		rolluptypes.EventTypeBurnETH,
		rolluptypes.AttributeKeyL2WithdrawalTx,
		rolluptypes.EventTypeWithdrawalInitiated,
	)
	page := 1
	perPage := 100
	orderBy := "desc"

	result, err := appchainClient.TxSearch(
		// TODO: use stack.Ctx here and for mint func?
		context.Background(),
		query,
		false,
		&page,
		&perPage,
		orderBy,
	)
	require.NoError(t, err, "search transactions")
	require.NotNil(t, result)
	require.NotEmpty(t, result.Txs, "burn_eth event not found")
	t.Log("Monomer can burn ETH for L2 user withdrawals")

	return big.NewInt(result.Txs[0].Height)
}
