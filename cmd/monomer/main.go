package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	bfttypes "github.com/cometbft/cometbft/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	rpcee "github.com/polymerdao/monomer/app/peptide/rpc_ee"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutil/testapp"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

type Config struct {
	DataDir    string           `json:"data_dir"`
	EthHost    string           `json:"eth_host"`
	EthPort    uint16           `json:"eth_port"`
	EngineHost string           `json:"engine_host"`
	EnginePort uint16           `json:"engine_port"`
	Genesis    *genesis.Genesis `json:"genesis"`
}

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current working directory: %v", err)
	}

	var dataDir string
	flag.StringVar(&dataDir, "data-dir", cwd, "")
	var engineHost string
	flag.StringVar(&engineHost, "engine-host", "127.0.0.1", "")
	var enginePort uint64
	flag.Uint64Var(&enginePort, "engine-port", 8888, "") //nolint:gomnd
	var ethHost string
	flag.StringVar(&ethHost, "eth-host", "127.0.0.1", "")
	var ethPort uint64
	flag.Uint64Var(&ethPort, "eth-port", 8889, "") //nolint:gomnd
	var genesisFile string
	flag.StringVar(&genesisFile, "genesis-file", "", "")

	flag.Parse()

	if enginePort > math.MaxUint16 {
		return nil, fmt.Errorf("engine port is out of range: %d", enginePort)
	} else if ethPort > math.MaxUint16 {
		return nil, fmt.Errorf("eth port is out of range: %d", ethPort)
	}

	g := new(genesis.Genesis)
	if genesisFile != "" {
		genesisBytes, err := os.ReadFile(genesisFile)
		if err != nil {
			return nil, fmt.Errorf("read genesis file: %v", err)
		}
		if err = json.Unmarshal(genesisBytes, &g); err != nil {
			return nil, fmt.Errorf("unmarshal genesis file: %v", err)
		}
	}

	return &Config{
		DataDir:    dataDir,
		EthHost:    ethHost,
		EthPort:    uint16(ethPort),
		EngineHost: engineHost,
		EnginePort: uint16(enginePort),
		Genesis:    g,
	}, nil
}

// run runs the Monomer node. It assumes args excludes the program name.
func run(ctx context.Context) error {
	config, err := parseFlags()
	if err != nil {
		return err
	}

	logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))

	appdb, err := openDB("app", config.DataDir)
	if err != nil {
		return err
	}
	app := testapp.New(appdb, config.Genesis.ChainID.String(), logger)

	blockdb, err := openDB("block", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close block db", blockdb.Close)
	}()
	blockStore := store.NewBlockStore(blockdb)

	if err = prepareBlockStoreAndApp(config.Genesis, blockStore, app); err != nil {
		return err
	}

	txdb, err := openDB("tx", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close tx db", txdb.Close)
	}()
	txStore := txstore.NewTxStore(txdb)

	mempooldb, err := openDB("mempool", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close mempool db", mempooldb.Close)
	}()
	mpool := mempool.New(mempooldb)

	eventBus := bfttypes.NewEventBus()

	ethAPI := struct {
		*eth.ChainID
		*eth.BlockByNumber
		*eth.BlockByHash
	}{
		ChainID:       eth.NewChainID(config.Genesis.ChainID.HexBig()),
		BlockByNumber: eth.NewBlockByNumber(blockStore, rolluptypes.AdaptCosmosTxsToEthTxs),
		BlockByHash:   eth.NewBlockByHash(blockStore, rolluptypes.AdaptCosmosTxsToEthTxs),
	}
	n := newNodeService(
		rpcee.NewEeRpcServer(config.EthHost, config.EthPort, []ethrpc.API{
			{
				Namespace: "eth",
				Service:   ethAPI,
			},
		}, logger),
		rpcee.NewEeRpcServer(config.EngineHost, config.EnginePort, []ethrpc.API{
			{
				Namespace: "engine",
				Service: engine.NewEngineAPI(
					builder.New(mpool, app, blockStore, txStore, eventBus, config.Genesis.ChainID),
					app,
					rolluptypes.AdaptPayloadTxsToCosmosTxs,
					blockStore,
				),
			},
			{
				Namespace: "eth",
				Service:   ethAPI,
			},
		}, logger), eventBus)

	if err := n.Start(); err != nil {
		return fmt.Errorf("start node: %v", err)
	}
	<-ctx.Done()
	if err := n.Stop(); err != nil {
		return fmt.Errorf("stop node: %v", err)
	}
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

func openDB(name, dir string) (tmdb.DB, error) {
	db, err := tmdb.NewDB(name, tmdb.GoLevelDBBackend, dir)
	if err != nil {
		return nil, fmt.Errorf("new db: %v", err)
	}
	return db, nil
}

func runAndWrapOnError(existingErr error, msg string, fn func() error) error {
	if runErr := fn(); runErr != nil {
		if existingErr == nil {
			return runErr
		}
		runErr = fmt.Errorf("%s: %v", msg, runErr)
		return fmt.Errorf(`failed to run because "%v" with existing err "%v"`, runErr, existingErr)
	}
	return existingErr
}
