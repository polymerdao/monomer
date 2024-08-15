package builder_test

import (
	"context"
	"fmt"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmtpubsub "github.com/cometbft/cometbft/libs/pubsub"
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
	"github.com/polymerdao/monomer/testapp"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

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
			inclusionListTxs := append([][]byte{testutils.GenerateBlock(t).Txs[0]}, testapp.ToTxs(t, test.inclusionList)...)
			mempoolTxs := testapp.ToTxs(t, test.mempool)

			pool := mempool.New(testutils.NewMemDB(t))
			for _, tx := range mempoolTxs {
				require.NoError(t, pool.Enqueue(tx))
			}
			blockStore := testutils.NewLocalMemDB(t)
			txStore := txstore.NewTxStore(testutils.NewCometMemDB(t))
			ethstatedb := testutils.NewEthStateDB(t)

			var chainID monomer.ChainID
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
			// +1 because we want it to be buffered even when mempool and inclusion list are empty.
			subChannelLen := len(test.mempool) + len(test.inclusionList) + 1
			subscription, err := eventBus.Subscribe(context.Background(), "test", &queryAll{}, subChannelLen)
			require.NoError(t, err)

			require.NoError(t, g.Commit(context.Background(), app, blockStore, ethstatedb))

			b := builder.New(
				pool,
				app,
				blockStore,
				txStore,
				eventBus,
				g.ChainID,
				ethstatedb,
			)

			payload := &builder.Payload{
				InjectedTransactions: bfttypes.ToTxs(inclusionListTxs),
				GasLimit:             0,
				Timestamp:            g.Time + 1,
				NoTxPool:             test.noTxPool,
			}
			preBuildInfo, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
			require.NoError(t, err)
			builtBlock, err := b.Build(context.Background(), payload)
			require.NoError(t, err)
			postBuildInfo, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
			require.NoError(t, err)

			// Application.
			{
				height := uint64(postBuildInfo.GetLastBlockHeight())
				app.StateContains(t, height, test.inclusionList)
				if test.noTxPool {
					app.StateDoesNotContain(t, height, test.mempool)
				} else {
					app.StateContains(t, height, test.mempool)
				}
			}

			// Block store.
			genesisHeader, err := blockStore.BlockByHeight(uint64(preBuildInfo.GetLastBlockHeight()))
			require.NoError(t, err)
			gotBlock, err := blockStore.HeadBlock()
			require.NoError(t, err)
			allTxs := append([][]byte{}, inclusionListTxs...)
			if !test.noTxPool {
				allTxs = append(allTxs, mempoolTxs...)
			}

			ethStateRoot := gotBlock.Header.StateRoot
			header := &monomer.Header{
				ChainID:    g.ChainID,
				Height:     uint64(postBuildInfo.GetLastBlockHeight()),
				Time:       payload.Timestamp,
				ParentHash: genesisHeader.Header.Hash,
				StateRoot:  ethStateRoot,
				GasLimit:   payload.GasLimit,
			}
			wantBlock, err := monomer.MakeBlock(header, bfttypes.ToTxs(allTxs))
			require.NoError(t, err)
			require.Equal(t, wantBlock, builtBlock)
			require.Equal(t, wantBlock, gotBlock)

			// Eth state db.
			ethState, err := state.New(ethStateRoot, ethstatedb, nil)
			require.NoError(t, err)
			appHash, err := getAppHashFromEVM(ethState, header)
			require.NoError(t, err)
			require.Equal(t, appHash[:], postBuildInfo.GetLastBlockAppHash())

			// Tx store and event bus.
			for i, tx := range wantBlock.Txs {
				checkTxResult := func(got abcitypes.TxResult) {
					// We don't check the full result, which would be difficult and a bit overkill.
					// We only verify that the main info is correct.
					require.Equal(t, uint32(i), got.Index)
					require.Equal(t, wantBlock.Header.Height, uint64(got.Height))
					require.Equal(t, tx, bfttypes.Tx(got.Tx))
				}

				// Tx store.
				got, err := txStore.Get(tx.Hash())
				require.NoError(t, err)
				checkTxResult(*got)

				// Event bus.
				select {
				case event, ok := <-subscription.Out():
					if !ok {
						require.FailNow(t, "event channel closed unexpectedly")
					}
					data := event.Data()
					require.IsType(t, bfttypes.EventDataTx{}, data)
					checkTxResult(data.(bfttypes.EventDataTx).TxResult)
				case <-subscription.Canceled():
					require.FailNow(t, "subscription channel closed unexpectedly")
				}
			}
			require.NoError(t, subscription.Err())
		})
	}
}

func TestRollback(t *testing.T) {
	pool := mempool.New(testutils.NewMemDB(t))
	blockStore := testutils.NewLocalMemDB(t)
	txStore := txstore.NewTxStore(testutils.NewCometMemDB(t))
	ethstatedb := testutils.NewEthStateDB(t)

	var chainID monomer.ChainID
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
	genesisHeader, err := blockStore.HeadHeader()
	require.NoError(t, err)

	b := builder.New(
		pool,
		app,
		blockStore,
		txStore,
		eventBus,
		g.ChainID,
		ethstatedb,
	)

	kvs := map[string]string{
		"test": "test",
	}
	block, err := b.Build(context.Background(), &builder.Payload{
		Timestamp:            g.Time + 1,
		InjectedTransactions: bfttypes.ToTxs(append([][]byte{testutils.GenerateBlock(t).Txs[0]}, testapp.ToTxs(t, kvs)...)),
	})
	require.NoError(t, err)
	require.NotNil(t, block)
	require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))

	// Eth state db before rollback.
	ethState, err := state.New(block.Header.StateRoot, ethstatedb, nil)
	require.NoError(t, err)
	require.NotEqual(t, ethState.GetStorageRoot(contracts.L2ApplicationStateRootProviderAddr), gethtypes.EmptyRootHash)

	// Rollback to genesis block.
	require.NoError(t, b.Rollback(context.Background(), genesisHeader.Hash, genesisHeader.Hash, genesisHeader.Hash))

	// Application.
	for k := range kvs {
		resp, err := app.Query(context.Background(), &abcitypes.RequestQuery{
			Data: []byte(k),
		})
		require.NoError(t, err)
		require.Empty(t, resp.GetValue()) // Value was removed from state.
	}

	// Block store.
	height, err := blockStore.Height()
	require.NoError(t, err)
	require.Equal(t, genesisHeader.Height, height)
	// We trust that the other parts of a block store rollback were done as well.

	// Eth state db after rollback.
	ethState, err = state.New(genesisHeader.StateRoot, ethstatedb, nil)
	require.NoError(t, err)
	require.Equal(t, ethState.GetStorageRoot(contracts.L2ApplicationStateRootProviderAddr), gethtypes.EmptyRootHash)

	// Tx store.
	for _, tx := range bfttypes.ToTxs(testapp.ToTxs(t, kvs)) {
		result, err := txStore.Get(tx.Hash())
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
