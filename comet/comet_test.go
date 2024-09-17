package comet_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/p2p"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	jsonrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/comet"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testapp"
	testmoduletypes "github.com/polymerdao/monomer/testapp/x/testmodule/types"
	"github.com/polymerdao/monomer/testutils"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestABCI(t *testing.T) {
	chainID := "0"
	app := testapp.NewTest(t, chainID)
	_, err := app.InitChain(context.Background(), &abcitypes.RequestInitChain{
		ChainId: chainID,
		AppStateBytes: func() []byte {
			appStateBytes, err := json.Marshal(testapp.MakeGenesisAppState(t, app))
			require.NoError(t, err)
			return appStateBytes
		}(),
	})
	require.NoError(t, err)

	// data to store and retrieve
	k := "k1"
	v := "v1"

	// Build bock with tx.
	height := int64(1)
	_, err = app.FinalizeBlock(context.Background(), &abcitypes.RequestFinalizeBlock{
		Txs:    [][]byte{testapp.ToTestTx(t, k, v)},
		Height: height,
	})
	require.NoError(t, err)
	_, err = app.Commit(context.Background(), &abcitypes.RequestCommit{})
	require.NoError(t, err)

	abci := comet.NewABCI(app)

	// Info.
	infoResult, err := abci.Info(&jsonrpctypes.Context{})
	require.NoError(t, err)
	require.Equal(t, height, infoResult.Response.LastBlockHeight) // We trust that the other fields are set properly.
	requestBytes, err := (&testmoduletypes.QueryValueRequest{
		Key: k,
	}).Marshal()
	require.NoError(t, err)

	// Query.
	queryResult, err := abci.Query(&jsonrpctypes.Context{}, testapp.QueryPath, requestBytes, height, false)
	require.NoError(t, err)
	var val testmoduletypes.QueryValueResponse
	require.NoError(t, (&val).Unmarshal(queryResult.Response.GetValue()))
	require.Equal(t, v, val.GetValue())
}

func TestStatus(t *testing.T) {
	blockStore := testutils.NewLocalMemDB(t)
	headBlock, err := monomer.MakeBlock(&monomer.Header{}, bfttypes.Txs{})
	require.NoError(t, err)
	require.NoError(t, blockStore.AppendBlock(headBlock))
	headCometBlock := headBlock.ToCometLikeBlock()
	startBlock := &bfttypes.Block{
		Header: bfttypes.Header{
			AppHash: []byte{},
			Height:  1,
			Time:    time.Now(),
		},
	}
	statusAPI := comet.NewStatus(blockStore, startBlock)
	result, err := statusAPI.Status(&jsonrpctypes.Context{})
	require.NoError(t, err)
	require.Equal(t, &rpctypes.ResultStatus{
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID:   "",
			ListenAddr:      "",
			Network:         headCometBlock.ChainID,
			Version:         version.TMCoreSemVer,
			Channels:        []byte("0123456789"),
			Moniker:         "monomer",
			ProtocolVersion: p2p.NewProtocolVersion(version.P2PProtocol, version.BlockProtocol, 0),
		},
		// We need SyncInfo so the CosmJS tmClient doesn't complain.
		SyncInfo: rpctypes.SyncInfo{
			LatestBlockHash:   headCometBlock.Hash(),
			LatestAppHash:     headCometBlock.AppHash,
			LatestBlockHeight: headCometBlock.Height,
			LatestBlockTime:   headCometBlock.Time,

			EarliestBlockHash:   startBlock.Hash(),
			EarliestAppHash:     startBlock.AppHash,
			EarliestBlockHeight: startBlock.Height,
			EarliestBlockTime:   startBlock.Time,

			CatchingUp: false,
		},
		ValidatorInfo: rpctypes.ValidatorInfo{},
	}, result)
}

func TestBroadcastTx(t *testing.T) {
	chainID := "0"
	app := testapp.NewTest(t, chainID)
	mpool := mempool.New(testutils.NewMemDB(t))
	broadcastTxAPI := comet.NewBroadcastTxAPI(app, mpool)

	// Success case.
	tx := testapp.ToTestTx(t, "k1", "v1")
	result, err := broadcastTxAPI.BroadcastTx(&jsonrpctypes.Context{}, tx)
	require.NoError(t, err)
	// We trust that the other fields are set correctly.
	require.Equal(t, uint32(0), result.Code)
	require.Equal(t, bfttypes.Tx(tx).Hash(), result.Hash.Bytes())
	// NOTE: I don't think there is a way to check if CheckTx has been run.
	// Both CheckTxType_New and CheckTxType_Recheck succeed.
	got, err := mpool.Dequeue()
	require.NoError(t, err)
	require.Equal(t, bfttypes.Tx(tx), got)

	// Error case - malformed tx.
	badTx := []byte{1, 2, 3}
	startLen, err := mpool.Len()
	require.NoError(t, err)

	result, err = broadcastTxAPI.BroadcastTx(&jsonrpctypes.Context{}, badTx)
	// API does not error, but returns a non-zero error code.
	require.NoError(t, err)
	require.NotEqual(t, uint32(0), result.Code)

	endLen, err := mpool.Len()
	require.NoError(t, err)
	// assert no insertion to mempool
	require.Equal(t, startLen, endLen)
}

type mockWSConnection struct {
	ctx    context.Context
	t      *testing.T
	writes chan<- jsonrpctypes.RPCResponse
}

func newMockWSConnection(t *testing.T, writes chan<- jsonrpctypes.RPCResponse) *mockWSConnection {
	return &mockWSConnection{
		ctx:    context.Background(),
		t:      t,
		writes: writes,
	}
}

func (c *mockWSConnection) GetRemoteAddr() string {
	return "ws://127.0.0.1:8888/ws"
}

func (c *mockWSConnection) WriteRPCResponse(_ context.Context, resp jsonrpctypes.RPCResponse) error {
	if c.writes != nil {
		c.writes <- resp
	}
	return nil
}

func (c *mockWSConnection) TryWriteRPCResponse(resp jsonrpctypes.RPCResponse) bool {
	// We know this is only called when writing an error string.
	var errStr string
	require.NoError(c.t, json.Unmarshal(resp.Result, &errStr))
	require.Contains(c.t, errStr, "subscription was canceled (reason")
	return true
}

func (c *mockWSConnection) Context() context.Context {
	return c.ctx
}

func TestSubscribeUnsubscribe(t *testing.T) {
	bus := bfttypes.NewEventBus()
	require.NoError(t, bus.Start())
	defer func() {
		require.NoError(t, bus.Stop())
	}()
	wg := conc.NewWaitGroup()
	defer wg.Wait()
	subscribeAPI := comet.NewSubscriberAPI(bus, wg, &comet.SelectiveListener{
		OnSubscriptionWriteErrCb: func(err error) {
			require.NoError(t, err)
		},
		OnSubscriptionCanceledCb: func(err error) {
			require.NoError(t, err)
		},
	})

	const height = 42
	query := fmt.Sprintf("tx.height = %d", height)

	// Subscribe, receive event, and unsubscribe.
	writes := make(chan jsonrpctypes.RPCResponse)
	defer close(writes)
	wsConn := newMockWSConnection(t, writes)
	result, err := subscribeAPI.Subscribe(&jsonrpctypes.Context{
		JSONReq: &jsonrpctypes.RPCRequest{
			JSONRPC: "2.0",
			ID:      jsonrpctypes.JSONRPCIntID(1),
			Method:  "subscribe",
		},
		WSConn: wsConn,
	}, query)
	require.NoError(t, err)
	require.Equal(t, &rpctypes.ResultSubscribe{}, result)
	event := bfttypes.EventDataTx{
		TxResult: abcitypes.TxResult{
			Height: height,
		},
	}
	require.NoError(t, bus.PublishEventTx(event))
	gotEvent := new(rpctypes.ResultEvent)
	require.NoError(t, json.Unmarshal((<-writes).Result, &gotEvent))
	gotHeight := func() int64 {
		x1, ok := gotEvent.Data.(map[string]any)
		require.True(t, ok)
		x2, ok := x1["value"].(map[string]any)
		require.True(t, ok)
		x3, ok := x2["TxResult"].(map[string]any)
		require.True(t, ok)
		x4, ok := x3["height"].(string)
		require.True(t, ok)
		got, err := strconv.ParseInt(x4, 10, 64)
		require.NoError(t, err)
		return got
	}()
	require.Equal(t, event.GetHeight(), gotHeight)
	resultUnsubscribe, err := subscribeAPI.Unsubscribe(&jsonrpctypes.Context{
		JSONReq: &jsonrpctypes.RPCRequest{
			JSONRPC: "2.0",
			ID:      jsonrpctypes.JSONRPCIntID(0),
			Method:  "unsubscribe",
		},
		WSConn: wsConn,
	}, query)
	require.NoError(t, err)
	require.Equal(t, &rpctypes.ResultUnsubscribe{}, resultUnsubscribe)

	// Subscribe multiple times and then unsubscribe via UnsubscribeAll.
	wsConn = newMockWSConnection(t, nil)
	_, err = subscribeAPI.Subscribe(&jsonrpctypes.Context{
		JSONReq: &jsonrpctypes.RPCRequest{
			JSONRPC: "2.0",
			ID:      jsonrpctypes.JSONRPCIntID(1),
			Method:  "subscribe",
		},
		WSConn: wsConn,
	}, query)
	require.NoError(t, err)
	_, err = subscribeAPI.Subscribe(&jsonrpctypes.Context{
		JSONReq: &jsonrpctypes.RPCRequest{
			JSONRPC: "2.0",
			ID:      jsonrpctypes.JSONRPCIntID(2),
			Method:  "subscribe",
		},
		WSConn: wsConn,
	}, "tx.index = 43")
	require.NoError(t, err)
	resultUnsubscribe, err = subscribeAPI.UnsubscribeAll(&jsonrpctypes.Context{
		JSONReq: &jsonrpctypes.RPCRequest{
			JSONRPC: "2.0",
			ID:      jsonrpctypes.JSONRPCIntID(3),
			Method:  "unsubscribe",
		},
		WSConn: wsConn,
	})
	require.NoError(t, err)
	require.Equal(t, &rpctypes.ResultUnsubscribe{}, resultUnsubscribe)
}

func TestTx(t *testing.T) {
	txStore := txstore.NewTxStore(testutils.NewCometMemDB(t))
	txAPI := comet.NewTxAPI(txStore)
	_, err := txAPI.ByHash(&jsonrpctypes.Context{}, []byte{}, true)
	require.ErrorContains(t, err, "proving is not supported")
	wsConn := newMockWSConnection(t, nil)
	_, err = txAPI.Search(&jsonrpctypes.Context{
		WSConn: wsConn,
	}, "tx.index = 1", true, nil, nil, "")
	require.ErrorContains(t, err, "proving is not supported")

	txResult1 := &abcitypes.TxResult{
		Height: 2,
		Index:  1,
		Tx:     []byte{1, 2, 3},
	}
	txResult2 := &abcitypes.TxResult{
		Height: txResult1.Height,
		Index:  txResult1.Index + 1,
		Tx:     []byte{4, 5, 6},
	}
	require.NoError(t, txStore.Add([]*abcitypes.TxResult{txResult1, txResult2}))

	resultTx, err := txAPI.ByHash(&jsonrpctypes.Context{}, bfttypes.Tx(txResult1.GetTx()).Hash(), false)
	require.NoError(t, err)
	// We trust that the other fields are set properly.
	require.Equal(t, txResult1.Height, resultTx.Height)
	require.Equal(t, txResult1.Index, resultTx.Index)

	// Ascending is the default, so the empty string also works.
	for _, orderAscending := range []string{"asc", ""} {
		searchResult, err := txAPI.Search(&jsonrpctypes.Context{
			WSConn: newMockWSConnection(t, nil),
		}, "tx.height = 2", false, nil, nil, orderAscending)
		require.NoError(t, err)
		require.Len(t, searchResult.Txs, 2)
		require.Equal(t, txResult1.Tx, []byte(searchResult.Txs[0].Tx))
		require.Equal(t, txResult2.Tx, []byte(searchResult.Txs[1].Tx))
	}

	searchResult, err := txAPI.Search(&jsonrpctypes.Context{
		WSConn: wsConn,
	}, "tx.height = 2", false, nil, nil, "desc")
	require.NoError(t, err)
	require.Len(t, searchResult.Txs, 2)
	require.Equal(t, txResult2.Tx, []byte(searchResult.Txs[0].Tx))
	require.Equal(t, txResult1.Tx, []byte(searchResult.Txs[1].Tx))
}

func TestBlock(t *testing.T) {
	blockStore := testutils.NewLocalMemDB(t)
	block, err := monomer.MakeBlock(&monomer.Header{
		Height: 3,
	}, bfttypes.Txs{})
	require.NoError(t, err)
	cometBlock := block.ToCometLikeBlock()
	want := &rpctypes.ResultBlock{
		BlockID: bfttypes.BlockID{
			Hash: cometBlock.Hash(),
		},
		Block: cometBlock,
	}
	require.NoError(t, blockStore.AppendBlock(block))

	blockAPI := comet.NewBlockAPI(blockStore)
	resultBlock, err := blockAPI.ByHeight(&jsonrpctypes.Context{}, int64(block.Header.Height))
	require.NoError(t, err)
	require.Equal(t, want, resultBlock)

	resultBlock, err = blockAPI.ByHash(&jsonrpctypes.Context{}, block.Header.Hash.Bytes())
	require.NoError(t, err)
	require.Equal(t, want, resultBlock)
}
