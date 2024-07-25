package node

import (
	"context"
	"fmt"
	"net"
	"net/http"

	cometdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	cometserver "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	jsonrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/core/state"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/comet"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/sourcegraph/conc"
)

type EventListener interface {
	OnEngineHTTPServeErr(error)
	OnEngineWebsocketServeErr(error)
	OnCometServeErr(error)
	OnPrometheusServeErr(error)
}

type Node struct {
	app            monomer.Application
	genesis        *genesis.Genesis
	engineWS       net.Listener
	cometHTTPAndWS net.Listener
	blockdb        dbm.DB
	txdb           cometdb.DB
	mempooldb      dbm.DB
	ethstatedb     *state.StateDB
	prometheusCfg  *config.InstrumentationConfig
	eventListener  EventListener
}

func New(
	app monomer.Application,
	g *genesis.Genesis,
	engineWS net.Listener,
	cometHTTPAndWS net.Listener,
	blockdb,
	mempooldb dbm.DB,
	txdb cometdb.DB,
	ethstatedb *state.StateDB,
	prometheusCfg *config.InstrumentationConfig,
	eventListener EventListener,
) *Node {
	return &Node{
		app:            app,
		genesis:        g,
		engineWS:       engineWS,
		cometHTTPAndWS: cometHTTPAndWS,
		blockdb:        blockdb,
		txdb:           txdb,
		ethstatedb:     ethstatedb,
		mempooldb:      mempooldb,
		prometheusCfg:  prometheusCfg,
		eventListener:  eventListener,
	}
}

func (n *Node) Run(ctx context.Context, env *environment.Env) error {
	ethStateTrieID := trie.StateTrieID(gethtypes.EmptyRootHash)
	ethStateTrie, err := trie.NewStateTrie(ethStateTrieID, n.ethstatedb.Database().TrieDB())
	if err != nil {
		return fmt.Errorf("new eth state trie: %v", err)
	}

	blockStore := store.NewBlockStore(n.blockdb)
	if err := prepareBlockStoreAndApp(ctx, n.genesis, blockStore, ethStateTrie, n.app); err != nil {
		return err
	}
	txStore := txstore.NewTxStore(n.txdb)
	mpool := mempool.New(n.mempooldb)

	eventBus := bfttypes.NewEventBus()
	if err := eventBus.Start(); err != nil {
		return fmt.Errorf("start event bus: %v", err)
	}
	env.DeferErr("stop event bus", eventBus.Stop)

	if err := n.startPrometheusServer(ctx, env); err != nil {
		return err
	}

	ethMetrics, engineMetrics := n.registerMetrics()

	rpcServer := rpc.NewServer()
	for _, api := range []rpc.API{
		{
			Namespace: "engine",
			Service: engine.NewEngineAPI(
				builder.New(mpool, n.app, blockStore, txStore, eventBus, n.genesis.ChainID, ethStateTrie),
				n.app,
				blockStore,
				engineMetrics,
			),
		},
		{
			Namespace: "eth",
			Service: struct {
				*eth.ChainID
				*eth.Block
			}{
				ChainID: eth.NewChainID(n.genesis.ChainID.HexBig(), ethMetrics),
				Block:   eth.NewBlock(blockStore, ethMetrics),
			},
		},
	} {
		if err := rpcServer.RegisterName(api.Namespace, api.Service); err != nil {
			return fmt.Errorf("register %s API: %v", api.Namespace, err)
		}
	}

	engineWS := makeHTTPService(rpcServer.WebsocketHandler([]string{}), n.engineWS)
	env.Go(func() {
		if err := engineWS.Run(ctx); err != nil {
			n.eventListener.OnEngineWebsocketServeErr(fmt.Errorf("run engine ws server: %v", err))
		}
	})

	// Run Comet server.

	abci := comet.NewABCI(n.app)
	broadcastTxAPI := comet.NewBroadcastTxAPI(n.app, mpool)
	txAPI := comet.NewTxAPI(txStore)
	subscribeWg := conc.NewWaitGroup()
	env.Defer(subscribeWg.Wait)
	subscribeAPI := comet.NewSubscriberAPI(eventBus, subscribeWg, &comet.SelectiveListener{})
	blockAPI := comet.NewBlockAPI(blockStore)
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

		"broadcast_tx_sync":  cometserver.NewRPCFunc(broadcastTxAPI.BroadcastTx, "tx"),
		"broadcast_tx_async": cometserver.NewRPCFunc(broadcastTxAPI.BroadcastTx, "tx"),

		"tx":        cometserver.NewRPCFunc(txAPI.ByHash, "hash,prove"),
		"tx_search": cometserver.NewRPCFunc(txAPI.Search, "query,prove,page,per_page,order_by"),

		"subscribe":       cometserver.NewRPCFunc(subscribeAPI.Subscribe, "query"),
		"unsubscribe":     cometserver.NewRPCFunc(subscribeAPI.Unsubscribe, "query"),
		"unsubscribe_all": cometserver.NewRPCFunc(subscribeAPI.UnsubscribeAll, ""),

		"block":         cometserver.NewRPCFunc(blockAPI.ByHeight, "height"),
		"block_by_hash": cometserver.NewRPCFunc(blockAPI.ByHash, "hash"),
	}

	cometMux := http.NewServeMux()
	cometserver.RegisterRPCFuncs(cometMux, routes, log.NewNopLogger())
	// We want to match cometbft's behavior, which puts the websocket endpoints under the /websocket route.
	cometMux.HandleFunc("/websocket", cometserver.NewWebsocketManager(routes).WebsocketHandler)
	cometServer := makeHTTPService(cometMux, n.cometHTTPAndWS)
	env.Go(func() {
		if err := cometServer.Run(ctx); err != nil {
			n.eventListener.OnCometServeErr(fmt.Errorf("run comet server: %v", err))
		}
	})

	return nil
}

func prepareBlockStoreAndApp(
	ctx context.Context,
	g *genesis.Genesis,
	blockStore store.BlockStore,
	ethStateTrie *trie.StateTrie,
	app monomer.Application,
) error {
	// Get blockStoreHeight and appHeight.
	var blockStoreHeight uint64
	if headBlock := blockStore.HeadBlock(); headBlock != nil {
		blockStoreHeight = uint64(headBlock.Header.Height)
	}
	info, err := app.Info(ctx, &abcitypes.RequestInfo{})
	if err != nil {
		return fmt.Errorf("info: %v", err)
	}
	appHeight := uint64(info.GetLastBlockHeight())

	// Ensure appHeight == blockStoreHeight.
	if appHeight == blockStoreHeight+1 {
		// There is a possibility that we committed to the app and Monomer crashed before committing to the block store.
		if err := app.RollbackToHeight(ctx, blockStoreHeight); err != nil {
			return fmt.Errorf("rollback app: %v", err)
		}
	} else if appHeight > blockStoreHeight {
		return fmt.Errorf("app height %d is too far ahead of block store height %d", appHeight, blockStoreHeight)
	} else if appHeight < blockStoreHeight {
		return fmt.Errorf("app height %d is behind block store height %d", appHeight, blockStoreHeight)
	}
	// Commit genesis.
	if blockStoreHeight == 0 { // We know appHeight == blockStoreHeight at this point.
		if err := g.Commit(ctx, app, blockStore, ethStateTrie.Hash()); err != nil {
			return fmt.Errorf("commit genesis: %v", err)
		}
	}
	return nil
}
