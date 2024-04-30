package builder_test

import (
	"context"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmtpubsub "github.com/cometbft/cometbft/libs/pubsub"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutil"
	"github.com/polymerdao/monomer/testutil/testapp"
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
			inclusionListTxs := testapp.ToTxs(t, test.inclusionList)
			mempoolTxs := testapp.ToTxs(t, test.mempool)

			pool := mempool.New(testutil.NewMemDB(t))
			for _, tx := range mempoolTxs {
				require.NoError(t, pool.Enqueue(tx))
			}
			blockStore := store.NewBlockStore(testutil.NewMemDB(t))
			txStore := txstore.NewTxStore(testutil.NewMemDB(t))

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

			require.NoError(t, g.Commit(app, blockStore))

			b := builder.New(
				pool,
				app,
				blockStore,
				txStore,
				eventBus,
				g.ChainID,
			)

			payload := &builder.Payload{
				Transactions: bfttypes.ToTxs(inclusionListTxs),
				GasLimit:     0,
				Timestamp:    g.Time + 1,
				NoTxPool:     test.noTxPool,
			}
			preBuildInfo := app.Info(abcitypes.RequestInfo{})
			require.NoError(t, b.Build(payload))
			postBuildInfo := app.Info(abcitypes.RequestInfo{})

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
			genesisBlock := blockStore.BlockByNumber(preBuildInfo.GetLastBlockHeight())
			require.NotNil(t, genesisBlock)
			gotBlock := blockStore.HeadBlock()
			allTxs := append([][]byte{}, inclusionListTxs...)
			if !test.noTxPool {
				allTxs = append(allTxs, mempoolTxs...)
			}
			wantBlock := &monomer.Block{
				Header: &monomer.Header{
					ChainID:    g.ChainID,
					Height:     postBuildInfo.GetLastBlockHeight(),
					Time:       payload.Timestamp,
					ParentHash: genesisBlock.Hash(),
					AppHash:    preBuildInfo.GetLastBlockAppHash(),
					GasLimit:   payload.GasLimit,
				},
				Txs: bfttypes.ToTxs(allTxs),
			}
			wantBlock.Hash()
			require.Equal(t, wantBlock, gotBlock)

			// Tx store and event bus.
			eventChan := subscription.Out()
			require.Len(t, eventChan, len(wantBlock.Txs))
			for i, tx := range wantBlock.Txs {
				checkTxResult := func(got abcitypes.TxResult) {
					// We don't check the full result, which would be difficult and a bit overkill.
					// We only verify that the main info is correct.
					require.Equal(t, uint32(i), got.Index)
					require.Equal(t, wantBlock.Header.Height, got.Height)
					require.Equal(t, tx, bfttypes.Tx(got.Tx))
				}

				// Tx store.
				got, err := txStore.Get(tx.Hash())
				require.NoError(t, err)
				checkTxResult(*got)

				// Event bus.
				event := <-eventChan
				data := event.Data()
				require.IsType(t, bfttypes.EventDataTx{}, data)
				checkTxResult(data.(bfttypes.EventDataTx).TxResult)
			}
			require.NoError(t, subscription.Err())
		})
	}
}

func TestRollback(t *testing.T) {
	pool := mempool.New(testutil.NewMemDB(t))
	blockStore := store.NewBlockStore(testutil.NewMemDB(t))
	txStore := txstore.NewTxStore(testutil.NewMemDB(t))

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

	require.NoError(t, g.Commit(app, blockStore))
	genesisBlock := blockStore.HeadBlock()

	b := builder.New(
		pool,
		app,
		blockStore,
		txStore,
		eventBus,
		g.ChainID,
	)

	kvs := map[string]string{
		"test": "test",
	}
	require.NoError(t, b.Build(&builder.Payload{
		Timestamp:    g.Time + 1,
		Transactions: bfttypes.ToTxs(testapp.ToTxs(t, kvs)),
	}))
	block := blockStore.HeadBlock()
	require.NotNil(t, block)
	require.NoError(t, blockStore.UpdateLabel(eth.Unsafe, block.Hash()))
	require.NoError(t, blockStore.UpdateLabel(eth.Safe, block.Hash()))
	require.NoError(t, blockStore.UpdateLabel(eth.Finalized, block.Hash()))

	require.NoError(t, b.Rollback(genesisBlock.Hash(), genesisBlock.Hash(), genesisBlock.Hash()))

	// Application.
	for k := range kvs {
		resp := app.Query(abcitypes.RequestQuery{
			Data: []byte(k),
		})
		require.Empty(t, resp.GetValue()) // Value was removed from state.
	}

	// Block store.
	headBlock := blockStore.HeadBlock()
	require.NotNil(t, headBlock)
	require.Equal(t, uint64(genesisBlock.Header.Height), uint64(headBlock.Header.Height))
	// We trust that the other parts of a block store rollback were done as well.

	// Tx store.
	for _, tx := range bfttypes.ToTxs(testapp.ToTxs(t, kvs)) {
		result, err := txStore.Get(tx.Hash())
		require.NoError(t, err)
		require.Nil(t, result)
	}
	// We trust that the other parts of a tx store rollback were done as well.
}
