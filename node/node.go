package node

import (
	"context"
	"fmt"
	"net"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
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
	httpListener               net.Listener
	wsListener                 net.Listener
	adaptCosmosTxsToEthTxs     monomer.CosmosTxAdapter
	adaptPayloadTxsToCosmosTxs monomer.PayloadTxAdapter
}

func New(
	app monomer.Application,
	g *genesis.Genesis,
	httpListener,
	wsListener net.Listener,
	adaptCosmosTxsToEthTxs monomer.CosmosTxAdapter,
	adaptPayloadTxsToCosmosTxs monomer.PayloadTxAdapter,
) *Node {
	return &Node{
		app:                        app,
		genesis:                    g,
		httpListener:               httpListener,
		wsListener:                 wsListener,
		adaptPayloadTxsToCosmosTxs: adaptPayloadTxsToCosmosTxs,
		adaptCosmosTxsToEthTxs:     adaptCosmosTxsToEthTxs,
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
				n.adaptPayloadTxsToCosmosTxs,
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

	httpServer := makeHTTPService(rpcServer, n.httpListener)
	var wg conc.WaitGroup
	wg.Go(func() {
		if err := httpServer.Run(ctx); err != nil {
			cancel(fmt.Errorf("start http server: %v", err))
		}
	})

	wsServer := makeHTTPService(rpcServer.WebsocketHandler([]string{}), n.wsListener)
	wg.Go(func() {
		if err := wsServer.Run(ctx); err != nil {
			cancel(fmt.Errorf("start websocket server: %v", err))
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
