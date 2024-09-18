package builder_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"cosmossdk.io/math"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmtpubsub "github.com/cometbft/cometbft/libs/pubsub"
	tmtypes "github.com/cometbft/cometbft/proto/tendermint/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/contracts"
	"github.com/polymerdao/monomer/evm"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/testapp"
	"github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/utils"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

type testEnvironment struct {
	app        *testapp.App
	g          *genesis.Genesis
	pool       *mempool.Pool
	blockStore *localdb.DB
	txStore    txstore.TxStore
	ethstatedb state.Database
	eventBus   *bfttypes.EventBus
}

type queryAll struct{}

var _ cmtpubsub.Query = (*queryAll)(nil)

func (*queryAll) Matches(_ map[string][]string) (bool, error) {
	return true, nil
}

func (*queryAll) String() string {
	return "all"
}

func TestBuild(t *testing.T) {
	tests := map[string]struct {
		inclusionList map[string]string
		mempool       map[string]string
		noTxPool      bool
	}{
		"no txs": {},
		"txs in inclusion list": {
			inclusionList: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		"txs in mempool": {
			mempool: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		"txs in mempool and inclusion list": {
			inclusionList: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
			mempool: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		"txs in mempool and inclusion list with NoTxPool": {
			inclusionList: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
			mempool: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
			noTxPool: true,
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			// +4 because there are 3 block events and we want it to be buffered by 1 even when mempool and inclusion list are empty.
			subChannelLen := len(test.mempool) + len(test.inclusionList) + 4
			env, subscription := setupTestEnvironmentAndSubscribe(t, subChannelLen)

			inclusionListTxs := append([][]byte{testutils.GenerateBlock(t).Txs[0]}, testapp.ToTxs(t, test.inclusionList)...)
			mempoolTxs := testapp.ToTxs(t, test.mempool)

			for _, tx := range mempoolTxs {
				require.NoError(t, env.pool.Enqueue(tx))
			}

			b := builder.New(
				env.pool,
				env.app,
				env.blockStore,
				env.txStore,
				env.eventBus,
				env.g.ChainID,
				env.ethstatedb,
			)

			payload := &builder.Payload{
				InjectedTransactions: bfttypes.ToTxs(inclusionListTxs),
				GasLimit:             0,
				Timestamp:            env.g.Time + 1,
				NoTxPool:             test.noTxPool,
			}

			builtBlock, preBuildInfo, postBuildInfo := buildBlock(t, b, env.app, payload)

			// Application.
			{
				height := uint64(postBuildInfo.GetLastBlockHeight())
				env.app.StateContains(t, height, test.inclusionList)
				if test.noTxPool {
					env.app.StateDoesNotContain(t, height, test.mempool)
				} else {
					env.app.StateContains(t, height, test.mempool)
				}
			}

			// Block store.
			genesisHeader, err := env.blockStore.BlockByHeight(uint64(preBuildInfo.GetLastBlockHeight()))
			require.NoError(t, err)
			gotBlock, err := env.blockStore.HeadBlock()
			require.NoError(t, err)
			allTxs := append([][]byte{}, inclusionListTxs...)
			if !test.noTxPool {
				allTxs = append(allTxs, mempoolTxs...)
			}

			ethStateRoot := gotBlock.Header.StateRoot
			header := &monomer.Header{
				ChainID:    env.g.ChainID,
				Height:     uint64(postBuildInfo.GetLastBlockHeight()),
				Time:       payload.Timestamp,
				ParentHash: genesisHeader.Header.Hash,
				StateRoot:  ethStateRoot,
				GasLimit:   payload.GasLimit,
			}
			wantBlock, err := monomer.MakeBlock(header, bfttypes.ToTxs(allTxs))
			require.NoError(t, err)
			verifyBlockContent(t, wantBlock, gotBlock, builtBlock)

			// Eth state db.
			ethState, err := state.New(ethStateRoot, env.ethstatedb, nil)
			require.NoError(t, err)
			appHash, err := getAppHashFromEVM(ethState, header)
			require.NoError(t, err)
			require.Equal(t, appHash[:], postBuildInfo.GetLastBlockAppHash())

			// Tx store and event bus.
			expectedTxResults := make([]*abcitypes.ExecTxResult, 0, len(wantBlock.Txs))
			for i, tx := range wantBlock.Txs {
				// Tx store.
				got, err := env.txStore.Get(tx.Hash())
				require.NoError(t, err)
				checkTxResult(t, got, wantBlock, i, tx)

				// Event bus.
				eventDataTx := getEventData[bfttypes.EventDataTx](t, subscription)
				checkTxResult(t, &eventDataTx.TxResult, wantBlock, i, tx)

				expectedTxResults = append(expectedTxResults, &eventDataTx.TxResult.Result)
			}

			var expectedBlockEvents []abcitypes.Event

			// Ensure that EventDataNewBlockEvents is emitted with the correct attributes.
			require.Equal(t, bfttypes.EventDataNewBlockEvents{
				Height: int64(wantBlock.Header.Height),
				Events: expectedBlockEvents,
				NumTxs: int64(len(wantBlock.Txs)),
			}, getEventData[bfttypes.EventDataNewBlockEvents](t, subscription))

			// Ensure that EventDataNewBlock is emitted with the correct attributes.
			require.Equal(t, bfttypes.EventDataNewBlock{
				Block: wantBlock.ToCometLikeBlock(),
				BlockID: bfttypes.BlockID{
					Hash: wantBlock.Header.Hash.Bytes(),
				},
				ResultFinalizeBlock: abcitypes.ResponseFinalizeBlock{
					Events:                expectedBlockEvents,
					TxResults:             expectedTxResults,
					AppHash:               postBuildInfo.GetLastBlockAppHash(),
					ConsensusParamUpdates: &tmtypes.ConsensusParams{},
				},
			}, getEventData[bfttypes.EventDataNewBlock](t, subscription))

			// Ensure that EventDataNewBlockHeader is emitted with the correct attributes.
			require.Equal(t, bfttypes.EventDataNewBlockHeader{
				Header: *wantBlock.Header.ToComet(),
			}, getEventData[bfttypes.EventDataNewBlockHeader](t, subscription))

			require.NoError(t, subscription.Err())
		})
	}
}

func TestRollback(t *testing.T) {
	env := setupTestEnvironment(t)
	genesisHeader, err := env.blockStore.HeadHeader()
	require.NoError(t, err)

	b := builder.New(
		env.pool,
		env.app,
		env.blockStore,
		env.txStore,
		env.eventBus,
		env.g.ChainID,
		env.ethstatedb,
	)

	kvs := map[string]string{
		"test": "test",
	}
	block, err := b.Build(context.Background(), &builder.Payload{
		Timestamp:            env.g.Time + 1,
		InjectedTransactions: bfttypes.ToTxs(append([][]byte{testutils.GenerateBlock(t).Txs[0]}, testapp.ToTxs(t, kvs)...)),
	})
	require.NoError(t, err)
	require.NotNil(t, block)
	require.NoError(t, env.blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))

	// Eth state db before rollback.
	ethState, err := state.New(block.Header.StateRoot, env.ethstatedb, nil)
	require.NoError(t, err)
	require.NotEqual(t, ethState.GetStorageRoot(contracts.L2ApplicationStateRootProviderAddr), gethtypes.EmptyRootHash)

	// Rollback to genesis block.
	require.NoError(t, b.Rollback(context.Background(), genesisHeader.Hash, genesisHeader.Hash, genesisHeader.Hash))

	// Application.
	for k := range kvs {
		resp, err := env.app.Query(context.Background(), &abcitypes.RequestQuery{
			Data: []byte(k),
		})
		require.NoError(t, err)
		require.Empty(t, resp.GetValue()) // Value was removed from state.
	}

	// Block store.
	height, err := env.blockStore.Height()
	require.NoError(t, err)
	require.Equal(t, genesisHeader.Height, height)
	// We trust that the other parts of a block store rollback were done as well.

	// Eth state db after rollback.
	ethState, err = state.New(genesisHeader.StateRoot, env.ethstatedb, nil)
	require.NoError(t, err)
	require.Equal(t, ethState.GetStorageRoot(contracts.L2ApplicationStateRootProviderAddr), gethtypes.EmptyRootHash)

	// Tx store.
	for _, tx := range bfttypes.ToTxs(testapp.ToTxs(t, kvs)) {
		result, err := env.txStore.Get(tx.Hash())
		require.NoError(t, err)
		require.Nil(t, result)
	}
	// We trust that the other parts of a tx store rollback were done as well.
}

// getAppHashFromEVM retrieves the updated cosmos app hash from the monomer EVM state db.
func getAppHashFromEVM(ethState *state.StateDB, header *monomer.Header) (common.Hash, error) {
	monomerEVM, err := evm.NewEVM(ethState, header)
	if err != nil {
		return common.Hash{}, fmt.Errorf("new EVM: %v", err)
	}
	executer, err := bindings.NewL2ApplicationStateRootProviderExecuter(monomerEVM)
	if err != nil {
		return common.Hash{}, fmt.Errorf("new L2ApplicationStateRootProviderExecuter: %v", err)
	}

	appHash, err := executer.GetL2ApplicationStateRoot()
	if err != nil {
		return common.Hash{}, fmt.Errorf("set L2ApplicationStateRoot: %v", err)
	}

	return appHash, nil
}

func TestBuildRollupTxs(t *testing.T) {
	// 3 for block events, 2 for deposit and withdrawal txs, plus 1 as buffer
	subChannelLen := 6
	env, subscription := setupTestEnvironmentAndSubscribe(t, subChannelLen)
	generatedBlock := testutils.GenerateBlock(t)

	// Get the L1 info & deposit transactions (combined)
	depositTxs := generatedBlock.Txs[0]

	ethTxs, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositTxs})
	require.NoError(t, err)
	require.Len(t, ethTxs, 2, "Expected two Ethereum transactions: L1 attribute and deposit tx, got %d", len(ethTxs))

	depositTxETH := ethTxs[1]
	require.NotNil(t, depositTxETH.Mint())
	require.NotNil(t, depositTxETH.To(), "Deposit transaction must have a 'to' address")

	from, err := gethtypes.NewCancunSigner(depositTxETH.ChainId()).Sender(depositTxETH)
	require.NoError(t, err)
	cosmAddr := utils.EvmToCosmosAddress(from)
	withdrawalTx := testapp.ToTx(t, &types.MsgInitiateWithdrawal{
		Sender:   cosmAddr.String(),
		Target:   common.HexToAddress("0x12345abcde").String(),
		Value:    math.NewIntFromBigInt(depositTxETH.Mint()),
		GasLimit: big.NewInt(100_000).Bytes(),
		Data:     []byte{},
	})

	b := builder.New(
		env.pool,
		env.app,
		env.blockStore,
		env.txStore,
		env.eventBus,
		env.g.ChainID,
		env.ethstatedb,
	)

	// Prepare the payload
	txs := bfttypes.Txs{depositTxs, withdrawalTx}
	payload := &builder.Payload{
		InjectedTransactions: txs,
		GasLimit:             1000000000000,
		Timestamp:            env.g.Time + 1,
		NoTxPool:             true,
	}

	// Build a block with the payload
	builtBlock, preBuildInfo, postBuildInfo := buildBlock(t, b, env.app, payload)

	// Test deposit was received
	checkDepositTxResult(t, env.txStore, depositTxs, fmt.Sprintf("%sETH", depositTxETH.Mint().String()), cosmAddr.String())

	withdrawalTxResult, err := env.txStore.Get(bfttypes.Tx(withdrawalTx).Hash())
	require.NoError(t, err)
	require.NotNil(t, withdrawalTxResult)
	require.Truef(t, withdrawalTxResult.Result.IsOK(), "Expected the withdrawal transaction to be successful, but it failed")

	// Verify block creation
	genesisBlock, err := env.blockStore.BlockByHeight(uint64(preBuildInfo.GetLastBlockHeight()))
	require.NoError(t, err)
	gotBlock, err := env.blockStore.HeadBlock()
	require.NoError(t, err)

	ethStateRoot := gotBlock.Header.StateRoot
	header := monomer.Header{
		ChainID:    env.g.ChainID,
		Height:     uint64(postBuildInfo.GetLastBlockHeight()),
		Time:       payload.Timestamp,
		ParentHash: genesisBlock.Header.Hash,
		StateRoot:  ethStateRoot,
		GasLimit:   payload.GasLimit,
	}
	wantBlock, err := monomer.MakeBlock(&header, txs)
	require.NoError(t, err)
	verifyBlockContent(t, wantBlock, gotBlock, builtBlock)

	// We expect two transactions: one combined transaction (l1InfoTx + depositTxs) and one withdrawal transaction (withdrawalTx).
	require.Equal(t, 2, builtBlock.Txs.Len(), "Expected the built block to contain 2 transactions: depositTxs and withdrawalTx")

	// Test parseWithdrawalMessages
	checkWithdrawalTxResult(t, withdrawalTxResult)

	expectedStateRoot := wantBlock.Header.StateRoot
	gotStateRoot := gotBlock.Header.StateRoot
	require.Equal(t, expectedStateRoot, gotStateRoot, "Expected the built block to contain state root hash")

	// Verify deposit and withdrawal msg event
	expectedEvents := []string{
		"/rollup.v1.MsgApplyL1Txs",
		"/rollup.v1.MsgInitiateWithdrawal",
	}

	for _, expectedEvent := range expectedEvents {
		eventData := getEventData[bfttypes.EventDataTx](t, subscription)
		require.Equal(t, expectedEvent, eventData.Result.Events[0].Attributes[0].Value,
			"Unexpected event type")
	}
}

func setupTestEnvironment(t *testing.T) testEnvironment {
	var chainID monomer.ChainID
	pool := mempool.New(testutils.NewMemDB(t))
	blockStore := testutils.NewLocalMemDB(t)
	txStore := txstore.NewTxStore(testutils.NewCometMemDB(t))
	ethstatedb := testutils.NewEthStateDB(t)

	app := testapp.NewTest(t, chainID.String())
	g := &genesis.Genesis{
		ChainID:  chainID,
		AppState: testapp.MakeGenesisAppState(t, app),
	}

	eventBus := bfttypes.NewEventBus()
	require.NoError(t, eventBus.Start())
	t.Cleanup(func() {
		require.NoError(t, eventBus.Stop())
	})

	require.NoError(t, g.Commit(context.Background(), app, blockStore, ethstatedb))

	return testEnvironment{
		app:        app,
		g:          g,
		pool:       pool,
		blockStore: blockStore,
		txStore:    txStore,
		ethstatedb: ethstatedb,
		eventBus:   eventBus,
	}
}

func subscribeToEventBus(t *testing.T, eventBus *bfttypes.EventBus, subChannelLen int) bfttypes.Subscription {
	subscription, err := eventBus.Subscribe(context.Background(), "test", &queryAll{}, subChannelLen)
	require.NoError(t, err)
	return subscription
}

func setupTestEnvironmentAndSubscribe(t *testing.T, subChannelLen int) (testEnvironment, bfttypes.Subscription) {
	env := setupTestEnvironment(t)
	subscription := subscribeToEventBus(t, env.eventBus, subChannelLen)
	return env, subscription
}

func buildBlock(
	t *testing.T,
	b *builder.Builder,
	app monomer.Application,
	payload *builder.Payload,
) (*monomer.Block, *abcitypes.ResponseInfo, *abcitypes.ResponseInfo) {
	preBuildInfo, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
	require.NoError(t, err)

	builtBlock, err := b.Build(context.Background(), payload)
	require.NoError(t, err)

	postBuildInfo, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
	require.NoError(t, err)

	return builtBlock, preBuildInfo, postBuildInfo
}

func verifyBlockContent(t *testing.T, wantBlock, gotBlock, builtBlock *monomer.Block) {
	require.Equal(t, wantBlock, builtBlock)
	require.Equal(t, wantBlock, gotBlock)
}

func checkTxResult(t *testing.T, got *abcitypes.TxResult, wantBlock *monomer.Block, i int, tx bfttypes.Tx) {
	require.Equal(t, uint32(i), got.Index)
	require.Equal(t, wantBlock.Header.Height, uint64(got.Height))
	require.Equal(t, tx, bfttypes.Tx(got.Tx))
}

func checkWithdrawalTxResult(t *testing.T, withdrawalTxResult *abcitypes.TxResult) {
	var withdrawalEvent *abcitypes.Event
	for i := range withdrawalTxResult.Result.Events {
		event := &withdrawalTxResult.Result.Events[i]
		if event.Type == types.EventTypeWithdrawalInitiated {
			withdrawalEvent = event
			break
		}
	}
	require.NotNil(t, withdrawalEvent, "Expected to find a withdrawal_initiated event")
	require.NotEmpty(t, withdrawalEvent.Attributes, "Expected attributes to not be empty for withdrawal event")

	var nonceAttribute *abcitypes.EventAttribute
	for i := range withdrawalEvent.Attributes {
		attribute := &withdrawalEvent.Attributes[i]
		if attribute.Key == types.AttributeKeyNonce {
			nonceAttribute = attribute
			break
		}
	}
	require.NotNil(t, nonceAttribute, "Expected to find a withdrawal nonce attribute")
	require.NotEmpty(t, nonceAttribute.Value, "Withdrawal nonce value should not be empty")
}

func checkDepositTxResult(t *testing.T, txStore txstore.TxStore, depositTx bfttypes.Tx, expectedDepositAmount, cosmAddr string) {
	const (
		receiverAttributeIndex = 1
		valueAttributeIndex    = 2
	)
	hash := depositTx.Hash()
	depositTxResult, err := txStore.Get(hash)
	require.NoError(t, err, "Failed to get deposit transaction result")

	var MintEthEvent *abcitypes.Event
	for i, event := range depositTxResult.Result.Events {
		if event.Type == types.EventTypeMintETH {
			MintEthEvent = &depositTxResult.Result.Events[i]
			break
		}
	}

	require.NotNil(t, MintEthEvent, "Expected to find the second coin_received event")

	actualReceiver := MintEthEvent.Attributes[receiverAttributeIndex].Value
	actualDepositAmount := MintEthEvent.Attributes[valueAttributeIndex].Value

	decimalValue := new(big.Int)
	decimalValue.SetString(actualDepositAmount[2:], 16)

	actualDepositAmount = fmt.Sprintf("%sETH", decimalValue.String())

	require.Equal(t, cosmAddr, actualReceiver, "Receiver address mismatch")
	require.Equal(t, expectedDepositAmount, actualDepositAmount, "Deposit amount mismatch")
}

// getEventData retrieves event data from the subscription channel. The event data is retrieved in the order
// that it was published with the event bus.
func getEventData[T any](t *testing.T, subscription bfttypes.Subscription) T {
	var eventType T
	select {
	case event, ok := <-subscription.Out():
		require.True(t, ok, "Event channel closed unexpectedly")
		data, ok := event.Data().(T)
		require.True(t, ok, "Expected %T type", eventType)
		return data
	case <-subscription.Canceled():
		require.FailNow(t, "Subscription channel closed unexpectedly")
		return eventType // This line will never be reached due to FailNow, but it's needed for compilation
	}
}
