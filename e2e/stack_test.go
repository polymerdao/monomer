package e2e_test

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"time"

	authv1beta1 "cosmossdk.io/api/cosmos/auth/v1beta1"
	"cosmossdk.io/math"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	cometcore "github.com/cometbft/cometbft/rpc/core/types"
	bfttypes "github.com/cometbft/cometbft/types"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
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
	"github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/utils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
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
		l1BlockInfo, err := derive.L1BlockInfoFromBytes(&rollup.Config{
			EcotoneTime: utils.Ptr(uint64(0)), // TODO: hacky
		}, uint64(eventNewBlock.Block.Time.Unix()), ethTxs[0].Data())
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

func verifierSync(t *testing.T, stack *e2e.StackConfig) {
	sequencerBlock, err := stack.MonomerClient.BlockByNumber(stack.Ctx, nil)
	require.NoError(t, err)

	// Wait for the verifier node to sync with the sequencer node
	for i := 0; i < 10; i++ {
		verifierBlock, err := stack.VerifierClient.BlockByHash(stack.Ctx, sequencerBlock.Header().Hash())
		if verifierBlock != nil && err == nil {
			t.Log("Verifier node can sync with the sequencer node")
			return
		}

		err = stack.WaitL1(1)
		require.NoError(t, err)
	}
	require.Fail(t, "verifier node did not sync with the sequencer node")
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

	// We need to wait for one L1 block here due to a tricky error that only affects tests.
	//
	// **Introduction**
	//
	// Initially, the deposit was either:
	//	- Evicted from the mempool due to exceeding the block gas limit, or
	//	- Reverting due to being out of gas.
	//
	// After struggling to manually find a working gas price, we decided to use the `PadGasEstimate`
	// function (copied from the op-e2e package) to estimate the gas price via simulation.
	//
	// During simulation, the transaction started reverting with a different error:
	// `arithmetic underflow or overflow`. The OP contracts require Solidity versions above v0.8.0.
	// In Solidity v0.8.0 and higher, contracts automatically revert on arithmetic underflows and
	// overflows. Thus, our goal is to discover where the arithmetic error occurs and prevent it.
	//
	// **Discovery**
	//
	// We generate the `allocs-l1.json` file by running `make devnet-allocs` in the optimism repo or
	// submodule. The `devnet-allocs` target calls a python script that uses a [forge script command]
	// to call `Deploy.s.sol`, which deploys the Optimism contracts.
	//
	// `forge script` behaves differently depending on the presence of the `--fork-url` param:
	//	- With `--fork-url`: runs the script against the provided endpoint. For example, this means
	//	  that the `block.number` Solidity variable will be set to the head block according to the
	//	  node running behind the endpoint.
	//	- Without `--fork-url`: runs the script against an [in-memory Foundry backend]. This will set
	//	  the `block.number` Solidity variable to `1`, the first block after genesis.
	//
	// The Deploy.s.sol script depends on the value of block.number when [initializing the resource meter]
	// used to burn gas in the [guaranteed gas market], setting the gas market's `params.prevBlockNum` equal
	// to `1`. The gas market code will try to determine when it last updated its internal parameters by
	// comparing the block.number to the params.prevBlockNum. In our case, we see an underflow whenever
	// `block.number == 0`.
	//
	// (For some reason, it seems that the gas simulation is simulating on top of the current block's state
	// (block zero), rather than the next block, which would prevent the problem altogether; we haven't
	// found the root cause for this yet)
	//
	// **Prevention**
	//
	// Wait one L1 block in the test to ensure `block.number > 0`, so `block.number - params.prevBlockNum`
	// does not underflow. This is kind of a hacky solution: ideally, we deploy the contracts to a new L1
	// chain when spinning up the devnet. However, we would need to clone the optimism repo every time to
	// run `forge script`, so we leave a more stable solution as future work.
	//
	// [forge script command]: https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/bedrock-devnet/devnet/__init__.py#L147
	// [in-memory Foundry backend]: https://github.com/foundry-rs/foundry/blob/d2ed15d517a3af56fced592aa4a21df0293710c5/crates/script/src/lib.rs#L595-L598
	// [initializing the resource meter]: https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/packages/contracts-bedrock/src/L1/ResourceMetering.sol#L160
	// [guaranteed gas market]: https://specs.optimism.io/protocol/guaranteed-gas-market.html
	require.NoError(t, stack.WaitL1(1))

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

	requireExpectedBalanceAfterDeposit(t, stack, receipt, userETHAddress, balanceBeforeDeposit, depositAmount)

	userCosmosAddr, err := userCosmosETHAddress.Encode("e2e")
	require.NoError(t, err)
	requireEthIsMinted(t, stack, userCosmosAddr, hexutil.EncodeBig(depositAmount))

	t.Log("Monomer can ingest OptimismPortal user deposit txs from L1 and mint ETH on L2")

	// get the user's balance after the deposit has been processed
	balanceBeforeDeposit, err = stack.L1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	bridgeDepositAmount := big.NewInt(params.Ether * 2)
	bridgeDepositTx, err := stack.L1StandardBridge.DepositETHTo(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, bridgeDepositAmount),
		common.Address(userCosmosETHAddress),
		100_000,  // l2GasLimit,
		[]byte{}, // no data
	)
	require.NoError(t, err)
	receipt, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, bridgeDepositTx.Hash())
	require.NoError(t, err)

	requireExpectedBalanceAfterDeposit(t, stack, receipt, userETHAddress, balanceBeforeDeposit, bridgeDepositAmount)

	// wait for tx to be processed on L2
	require.NoError(t, stack.WaitL2(2))

	requireEthIsMinted(t, stack, userCosmosAddr, hexutil.EncodeBig(bridgeDepositAmount))

	t.Log("Monomer can ingest L1StandardBridge user deposit txs from L1 and mint ETH on L2")

	/////////////////////////////
	////// ETH WITHDRAWALS //////
	/////////////////////////////

	withdrawalAmount := big.NewInt(1)

	// create a withdrawal tx to withdraw the deposited amount from L2 back to L1
	withdrawalTx := e2e.NewWithdrawalTx(0, common.Address(userCosmosETHAddress), userETHAddress, withdrawalAmount, new(big.Int).SetUint64(params.TxGas))

	baseAccount := queryAccount(t, stack, userCosmosAddr)
	l2ChainID, err := stack.MonomerClient.ChainID(stack.Ctx)
	require.NoError(t, err)

	l2WithdrawalTxBytes, err := testutils.BuildSDKTx(t, l2ChainID.String(), baseAccount.Sequence, baseAccount.AccountNumber, userPrivKey, []protov1.Message{
		&rolluptypes.MsgInitiateWithdrawal{
			Sender:   userCosmosAddr,
			Target:   withdrawalTx.Target.String(),
			Value:    math.NewIntFromBigInt(withdrawalTx.Value),
			GasLimit: withdrawalTx.GasLimit.Bytes(),
			Data:     []byte{},
		},
	}).Marshal()
	require.NoError(t, err)
	withdrawalTxResult, err := stack.L2Client.BroadcastTxAsync(stack.Ctx, l2WithdrawalTxBytes)
	require.NoError(t, err)
	require.Equalf(t, abcitypes.CodeTypeOK, withdrawalTxResult.Code, "log: "+withdrawalTxResult.Log)

	// wait for tx to be processed on L2
	require.NoError(t, stack.WaitL2(2))

	// inspect L2 events to ensure that the user's ETH was burned on L2
	requireEthIsBurned(t, stack, userCosmosAddr, hexutil.EncodeBig(withdrawalAmount))

	// get the user's balance before the withdrawal has been finalized
	balanceBeforeFinalization, err := stack.L1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	gasCost := proveAndFinalizeWithdrawal(t, stack, userPrivKey, l1signer, withdrawalTx, l1Client)

	// get the user's balance after the withdrawal has been finalized
	balanceAfterFinalization, err := stack.L1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	//nolint:gocritic
	// expectedBalance = balanceBeforeFinalization + withdrawalAmount - gasCost
	expectedBalance := new(big.Int).Sub(new(big.Int).Add(balanceBeforeFinalization, withdrawalAmount), gasCost)

	// check that the user's balance has been updated on L1
	require.Equal(t, expectedBalance, balanceAfterFinalization)

	t.Log("Monomer can initiate withdrawals on L2 and can generate proofs for verifying the withdrawal on L1")
}

func erc20RollupFlow(t *testing.T, stack *e2e.StackConfig) {
	l1Client := stack.L1Client

	l1ChainID, err := l1Client.ChainID(stack.Ctx)
	require.NoError(t, err, "chain id")

	// instantiate L1 user, tx signer.
	userPrivKey := stack.Users[1]
	userEthAddress := ethcrypto.PubkeyToAddress(userPrivKey.PublicKey)
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
	userWethBalance, err := WETH9.BalanceOf(&bind.CallOpts{}, userEthAddress)
	require.NoError(t, err)
	require.Equal(t, wethL1Amount, userWethBalance)

	// approve WETH9 transfer with the L1StandardBridge address
	tx, err = WETH9.Approve(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		stack.L1Deployments.L1StandardBridgeProxy,
		wethL1Amount,
	)
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, tx.Hash())
	require.NoError(t, err)

	userCosmosETHAddr := monomer.PubkeyToCosmosETHAddress(&userPrivKey.PublicKey)

	// bridge the WETH9
	wethL2Amount := big.NewInt(100)
	tx, err = stack.L1StandardBridge.DepositERC20To(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, nil),
		weth9Address,
		weth9Address,
		common.Address(userCosmosETHAddr),
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
	userWethBalance, err = WETH9.BalanceOf(&bind.CallOpts{}, userEthAddress)
	require.NoError(t, err)
	require.Equal(t, new(big.Int).Sub(wethL1Amount, wethL2Amount), userWethBalance)

	// wait for tx to be processed
	// 1 L1 block to process the tx on L1 +
	// 1 L2 block to process the tx on L2
	require.NoError(t, stack.WaitL1(1))
	require.NoError(t, stack.WaitL2(1))

	// assert the user's bridged WETH is on L2
	userAddr, err := userCosmosETHAddr.Encode("e2e")
	require.NoError(t, err)
	requireERC20IsMinted(t, stack, userAddr, weth9Address.String(), hexutil.EncodeBig(wethL2Amount))

	t.Log("Monomer can ingest ERC-20 deposit txs from L1 and mint ERC-20 tokens on L2")

	////////////////////////////////
	////// ERC-20 WITHDRAWALS //////
	////////////////////////////////

	// deposit ETH to the user's account on L2 to pay for gas
	depositAmount := big.NewInt(params.Ether)
	depositTx, err := PadGasEstimate(
		createL1TransactOpts(t, stack, userPrivKey, l1signer, depositAmount),
		1.1,
		func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return stack.OptimismPortal.DepositTransaction(
				opts,
				common.Address(userCosmosETHAddr),
				depositAmount,
				100_000,  // l2GasLimit,
				false,    // _isCreation
				[]byte{}, // no data
			)
		},
	)
	require.NoError(t, err)

	// wait for ETH deposit tx to be processed
	_, err = wait.ForReceiptOK(stack.Ctx, l1Client.Client, depositTx.Hash())
	require.NoError(t, err, "waiting for deposit tx on L1")
	require.NoError(t, stack.WaitL2(2))

	// create a withdrawal tx to withdraw the deposited amount from L2 back to L1
	// the withdrawal tx nonce is set to 1 here because the ETH withdrawal tx test is expected to run before it and uses nonce 0
	withdrawalTx := e2e.NewWithdrawalTx(1, common.HexToAddress(predeploys.L2CrossDomainMessenger), common.HexToAddress(rolluptypes.DefaultParams().L1CrossDomainMessenger), big.NewInt(0), big.NewInt(200_000))

	baseAccount := queryAccount(t, stack, userAddr)
	l2ChainID, err := stack.MonomerClient.ChainID(stack.Ctx)
	require.NoError(t, err)

	l2WithdrawalTxBytes, err := testutils.BuildSDKTx(t, l2ChainID.String(), baseAccount.Sequence, baseAccount.AccountNumber, userPrivKey, []protov1.Message{
		&rolluptypes.MsgInitiateERC20Withdrawal{
			Sender:       userAddr,
			Target:       userEthAddress.String(),
			TokenAddress: weth9Address.String(),
			Value:        math.NewIntFromBigInt(wethL2Amount),
			GasLimit:     withdrawalTx.GasLimit.Bytes(),
			ExtraData:    nil,
		},
	}).Marshal()
	require.NoError(t, err)
	withdrawalTxResult, err := stack.L2Client.BroadcastTxAsync(stack.Ctx, l2WithdrawalTxBytes)
	require.NoError(t, err)
	require.Equalf(t, abcitypes.CodeTypeOK, withdrawalTxResult.Code, "log: "+withdrawalTxResult.Log)

	// wait for tx to be processed on L2
	require.NoError(t, stack.WaitL2(3))

	// inspect L2 events to ensure that the user's ETH was burned on L2 and add the withdrawal data to the withdrawal tx
	withdrawalTx.Data = requireERC20IsBurnedAndGetData(t, stack, userAddr, weth9Address.String(), hexutil.EncodeBig(wethL2Amount))

	proveAndFinalizeWithdrawal(t, stack, userPrivKey, l1signer, withdrawalTx, l1Client)

	// assert the user's L2 WETH has been bridged to L1
	userWethBalance, err = WETH9.BalanceOf(&bind.CallOpts{}, userEthAddress)
	require.NoError(t, err)
	require.Equal(t, wethL1Amount, userWethBalance)

	t.Log("Monomer can initiate ERC-20 withdrawals on L2 and can generate proofs for verifying the withdrawal on L1")
}

// proveAndFinalizeWithdrawal builds and submits a withdrawal proof and withdrawal proving tx on L1 and then submits a
// withdrawal finalizing tx on L1 after the finalization period has elapsed. The gas cost of the submitted txs is returned.
func proveAndFinalizeWithdrawal(t *testing.T, stack *e2e.StackConfig, userPrivKey *ecdsa.PrivateKey, l1signer types.Signer, withdrawalTx *crossdomain.Withdrawal, l1Client *e2e.L1Client) *big.Int {
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
	proveReceipt, err := l1Client.Client.TransactionReceipt(stack.Ctx, proveWithdrawalTx.Hash())
	require.NoError(t, err, "withdrawal proving tx receipt")
	require.NotNil(t, proveReceipt, "withdrawal proving tx receipt")
	require.Equal(t, types.ReceiptStatusSuccessful, proveReceipt.Status, "withdrawal proving tx failed")

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
	finalizeReceipt, err := l1Client.Client.TransactionReceipt(stack.Ctx, finalizeWithdrawalTx.Hash())
	require.NoError(t, err, "finalize withdrawal tx receipt")
	require.NotNil(t, finalizeReceipt, "finalize withdrawal tx receipt")
	require.Equal(t, types.ReceiptStatusSuccessful, finalizeReceipt.Status, "finalize withdrawal tx failed")

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

	return new(big.Int).Add(getGasCost(proveReceipt), getGasCost(finalizeReceipt))
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

// requireERC20IsBurnedAndGetData searches for a burn_erc20 event on L2 with the given user address, token address, and value.
// The ERC-20 withdrawal data is returned as well for use in proving and finalizing the withdrawal on L1.
func requireERC20IsBurnedAndGetData(t *testing.T, stack *e2e.StackConfig, userAddress, tokenAddress, valueHex string) []byte {
	query := fmt.Sprintf(
		"%s.%s='%s' AND %s.%s='%s' AND %s.%s='%s' AND %s.%s='%s'",
		rolluptypes.EventTypeBurnERC20, rolluptypes.AttributeKeyL2WithdrawalTx, rolluptypes.EventTypeWithdrawalInitiated,
		rolluptypes.EventTypeBurnERC20, rolluptypes.AttributeKeyFromCosmosAddress, userAddress,
		rolluptypes.EventTypeBurnERC20, rolluptypes.AttributeKeyERC20Address, tokenAddress,
		rolluptypes.EventTypeBurnERC20, rolluptypes.AttributeKeyValue, valueHex,
	)
	result := l2TxSearch(t, stack, query)

	require.NotEmpty(t, result.Txs, "burn_erc20 event not found")

	for _, event := range result.Txs[0].TxResult.Events {
		if event.Type == rolluptypes.EventTypeWithdrawalInitiated {
			for _, attr := range event.Attributes {
				if attr.Key == rolluptypes.AttributeKeyData {
					data, err := hexutil.Decode(attr.Value)
					require.NoError(t, err)
					return data
				}
			}
		}
	}
	require.Fail(t, "withdrawal event does not contain data attribute")

	return nil
}

func requireExpectedBalanceAfterDeposit(
	t *testing.T,
	stack *e2e.StackConfig,
	receipt *types.Receipt,
	userETHAddress common.Address,
	balanceBeforeDeposit, depositAmount *big.Int,
) {
	// check that the deposit tx went through the OptimismPortal successfully
	_, err := receipts.FindLog(receipt.Logs, stack.OptimismPortal.ParseTransactionDeposited)
	require.NoError(t, err, "should emit deposit event")

	// get the user's balance after the deposit has been processed
	balanceAfterDeposit, err := stack.L1Client.BalanceAt(stack.Ctx, userETHAddress, nil)
	require.NoError(t, err)

	//nolint:gocritic
	// expectedBalance = balanceBeforeDeposit - bridgeDepositAmount - gasCost
	expectedBalance := new(big.Int).Sub(new(big.Int).Sub(balanceBeforeDeposit, depositAmount), getGasCost(receipt))

	// check that the user's balance has been updated on L1
	require.Equal(t, expectedBalance, balanceAfterDeposit)
}

func getGasCost(receipt *types.Receipt) *big.Int {
	//nolint:gocritic
	// gasCost = gasUsed * gasPrice
	return new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
}

func queryAccount(t *testing.T, stack *e2e.StackConfig, address string) *authv1beta1.BaseAccount {
	queryAccountBytes, err := protov1.Marshal(&authv1beta1.QueryAccountRequest{
		Address: address,
	})
	require.NoError(t, err)
	queryResult, err := stack.L2Client.ABCIQuery(stack.Ctx, authv1beta1.Query_Account_FullMethodName, queryAccountBytes)
	require.NoError(t, err)
	require.Zero(t, queryResult.Response.Code, queryResult.Response.Log)
	var accountResponse authv1beta1.QueryAccountResponse
	require.NoError(t, protov1.Unmarshal(queryResult.Response.Value, &accountResponse))
	var baseAccount authv1beta1.BaseAccount
	require.NoError(t, protov1.Unmarshal(accountResponse.Account.Value, &baseAccount))
	return &baseAccount
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
