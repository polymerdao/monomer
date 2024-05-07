package node

import (
	"context"
	"fmt"
	"net"
	"net/http"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	cometserver "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	jsonrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/comet"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/utils"
	"github.com/sourcegraph/conc"
)

type Node struct {
	app                        monomer.Application
	genesis                    *genesis.Genesis
	engineHTTP                 net.Listener
	engineWS                   net.Listener
	cometHTTPAndWS             net.Listener
	adaptCosmosTxsToEthTxs     monomer.CosmosTxAdapter
	adaptPayloadTxsToCosmosTxs monomer.PayloadTxAdapter
}

func New(
	app monomer.Application,
	g *genesis.Genesis,
	engineHTTP net.Listener,
	engineWS net.Listener,
	cometHTTPAndWS net.Listener,
	adaptCosmosTxsToEthTxs monomer.CosmosTxAdapter,
	adaptPayloadTxsToCosmosTxs monomer.PayloadTxAdapter,
) *Node {
	return &Node{
		app:                        app,
		genesis:                    g,
		engineHTTP:                 engineHTTP,
		engineWS:                   engineWS,
		cometHTTPAndWS:             cometHTTPAndWS,
		adaptCosmosTxsToEthTxs:     adaptCosmosTxsToEthTxs,
		adaptPayloadTxsToCosmosTxs: adaptPayloadTxsToCosmosTxs,
	}
}

func (n *Node) Run(parentCtx context.Context) (err error) {
	ctx, cancel := context.WithCancelCause(parentCtx)
	defer func() {
		cancel(err)
		err = utils.Cause(ctx)
	}()

	blockdb := tmdb.NewMemDB()
	defer func() {
		err = utils.RunAndWrapOnError(err, "close block db", blockdb.Close)
	}()
	blockStore := store.NewBlockStore(blockdb)

	if err = prepareBlockStoreAndApp(n.genesis, blockStore, n.app); err != nil {
		return err
	}

	txdb := tmdb.NewMemDB()
	defer func() {
		err = utils.RunAndWrapOnError(err, "close tx db", txdb.Close)
	}()
	txStore := txstore.NewTxStore(txdb)

	mempooldb := tmdb.NewMemDB()
	defer func() {
		err = utils.RunAndWrapOnError(err, "close mempool db", mempooldb.Close)
	}()
	mpool := mempool.New(mempooldb)

	eventBus := bfttypes.NewEventBus()
	if err := eventBus.Start(); err != nil {
		return fmt.Errorf("start event bus: %v", err)
	}
	defer func() {
		err = utils.RunAndWrapOnError(err, "stop event bus", eventBus.Stop)
	}()

	rpcServer := rpc.NewServer()
	for _, api := range []rpc.API{
		{
			Namespace: "engine",
			Service: engine.NewEngineAPI(
				builder.New(mpool, n.app, blockStore, txStore, eventBus, n.genesis.ChainID),
				n.app,
				monomer.DuplexAdapter{
					EthToCosmos: n.adaptPayloadTxsToCosmosTxs,
					CosmosToEth: n.adaptCosmosTxsToEthTxs,
				},
				blockStore,
			),
		},
		{
			Namespace: "eth",
			Service: struct {
				*eth.ChainID
				*eth.Block
			}{
				ChainID: eth.NewChainID(n.genesis.ChainID.HexBig()),
				Block:   eth.NewBlock(blockStore, n.adaptCosmosTxsToEthTxs),
			},
		},
	} {
		if err := rpcServer.RegisterName(api.Namespace, api.Service); err != nil {
			return fmt.Errorf("register %s API: %v", api.Namespace, err)
		}
	}

	httpServer := makeHTTPService(rpcServer, n.engineHTTP)
	var wg conc.WaitGroup
	wg.Go(func() {
		if err := httpServer.Run(ctx); err != nil {
			cancel(fmt.Errorf("start engine http server: %v", err))
		}
	})

	wsServer := makeHTTPService(rpcServer.WebsocketHandler([]string{}), n.engineWS)
	wg.Go(func() {
		if err := wsServer.Run(ctx); err != nil {
			cancel(fmt.Errorf("start engine websocket server: %v", err))
		}
	})

	// Run Comet server.

	abci := comet.NewABCI(n.app)
	broadcastAPI := comet.NewBroadcastTx(n.app, mpool)
	txAPI := comet.NewTx(txStore)
	subscribeWg := conc.NewWaitGroup()
	defer subscribeWg.Wait()
	subscribeAPI := comet.NewSubscriber(eventBus, subscribeWg, &comet.SelectiveListener{})
	blockAPI := comet.NewBlock(blockStore)
	// https://docs.cometbft.com/main/rpc/
	routes := map[string]*cometserver.RPCFunc{
		"echo": cometserver.NewRPCFunc(func(_ *jsonrpctypes.Context, msg string) (string, error) {
			return msg, nil
		}, "msg"),
		"health": cometserver.NewRPCFunc(func(_ *jsonrpctypes.Context) (*rpctypes.ResultHealth, error) {
			return &rpctypes.ResultHealth{}, nil
		}, ""),
		"status": cometserver.NewRPCFunc(comet.NewStatus(blockStore, nil).Status, ""), // TODO start block

		"abci_query": cometserver.NewRPCFunc(abci.Query, "path,data,height,prove"),
		"abci_info":  cometserver.NewRPCFunc(abci.Info, "", cometserver.Cacheable()),

		"broadcast_tx_sync":  cometserver.NewRPCFunc(broadcastAPI.BroadcastTx, "tx"),
		"broadcast_tx_async": cometserver.NewRPCFunc(broadcastAPI.BroadcastTx, "tx"),

		"tx":        cometserver.NewRPCFunc(txAPI.ByHash, "hash,prove"),
		"tx_search": cometserver.NewRPCFunc(txAPI.Search, "query,prove,page,per_page,order_by"),

		"subscribe":       cometserver.NewRPCFunc(subscribeAPI.Subscribe, "query"),
		"unsubscribe":     cometserver.NewRPCFunc(subscribeAPI.Unsubscribe, "query"),
		"unsubscribe_all": cometserver.NewRPCFunc(subscribeAPI.UnsubscribeAll, ""),

		"block":         cometserver.NewRPCFunc(blockAPI.ByHeight, "height"),
		"block_by_hash": cometserver.NewRPCFunc(blockAPI.ByHash, "hash"),
	}

	mux := http.NewServeMux()
	cometserver.RegisterRPCFuncs(mux, routes, log.NewNopLogger())
	// We want to match cometbft's behavior, which puts the websocket endpoints under the /websocket route.
	mux.HandleFunc("/websocket", cometserver.NewWebsocketManager(routes).WebsocketHandler)

	cometServer := makeHTTPService(mux, n.cometHTTPAndWS)
	wg.Go(func() {
		if err := cometServer.Run(ctx); err != nil {
			cancel(fmt.Errorf("start comet server: %v", err))
		}
	})

	<-ctx.Done()
	return nil
}

func prepareBlockStoreAndApp(g *genesis.Genesis, blockStore store.BlockStore, app monomer.Application) error {
	// Get blockStoreHeight and appHeight.
	var blockStoreHeight uint64
	if headBlock := blockStore.HeadBlock(); headBlock != nil {
		blockStoreHeight = uint64(headBlock.Header.Height)
	}
	info := app.Info(abcitypes.RequestInfo{})
	appHeight := uint64(info.GetLastBlockHeight())

	// Ensure appHeight == blockStoreHeight.
	if appHeight == blockStoreHeight+1 {
		// There is a possibility that we committed to the app and Monomer crashed before committing to the block store.
		if err := app.RollbackToHeight(blockStoreHeight); err != nil {
			return fmt.Errorf("rollback app: %v", err)
		}
	} else if appHeight > blockStoreHeight {
		return fmt.Errorf("app height %d is too far ahead of block store height %d", appHeight, blockStoreHeight)
	} else if appHeight < blockStoreHeight {
		return fmt.Errorf("app height %d is behind block store height %d", appHeight, blockStoreHeight)
	}

	// Commit genesis.
	if blockStoreHeight == 0 { // We know appHeight == blockStoreHeight at this point.
		if err := g.Commit(app, blockStore); err != nil {
			return fmt.Errorf("commit genesis: %v", err)
		}
	}
	return nil
}
