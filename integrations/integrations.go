package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethclient"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

const (
	flagEngineURL         = "monomer.engine-url"
	flagDev               = "monomer.dev-start"
	flagL1AllocsPath      = "monomer.dev.l1-allocs"
	flagL1DeploymentsPath = "monomer.dev.l1-deployments"
	flagDeployConfigPath  = "monomer.dev.deploy-config"
	flagMneumonicsPath    = "monomer.dev.mneumonics"
	flagL1URL             = "monomer.dev.l1-url"
	flagOPNodeURL         = "monomer.dev.op-node-url"
	flagBatcherPrivKey    = "monomer.dev.batcher-privkey"
	flagProposerPrivKey   = "monomer.dev.proposer-privkey"

	defaultCacheSize   = 16 // 16 MB
	defaultHandlesSize = 16
)

var sigCh = make(chan os.Signal, 1)

func AddMonomerCommand(rootCmd *cobra.Command, appCreator servertypes.AppCreator, defaultNodeHome string) {
	monomerCmd := &cobra.Command{
		Use:   "monomer",
		Short: "Monomer subcommands",
	}
	monomerCmd.AddCommand(server.StartCmdWithOptions(appCreator, defaultNodeHome, server.StartCmdOptions{
		StartCommandHandler: startCommandHandler,
		AddFlags: func(cmd *cobra.Command) {
			cmd.Flags().String(flagEngineURL, "ws://127.0.0.1:9000", "url of Monomer's Engine API endpoint")
			cmd.Flags().Bool(flagDev, false, "run the OP Stack devnet in-process for testing")
			cmd.Flags().String(flagL1URL, "ws://127.0.0.1:9001", "")
			cmd.Flags().String(flagOPNodeURL, "http://127.0.0.1:9002", "")
			cmd.Flags().String(flagL1DeploymentsPath, "", "")
			cmd.Flags().String(flagDeployConfigPath, "", "")
			cmd.Flags().String(flagL1AllocsPath, "", "")
			cmd.Flags().String(flagMneumonicsPath, "", "")
		},
	}))
	rootCmd.AddCommand(monomerCmd)
}

// startCommandHandler is a custom callback that overrides the default `start` function in the Cosmos
// SDK. It starts a Monomer node in-process instead of a CometBFT node.
func startCommandHandler(
	svrCtx *server.Context,
	clientCtx client.Context, //nolint:gocritic // hugeParam
	appCreator servertypes.AppCreator,
	inProcessConsensus bool,
	opts server.StartCmdOptions,
) error {
	env := environment.New()
	defer func() {
		if err := env.Close(); err != nil {
			svrCtx.Logger.Error("Failed to close environment", "err", err)
		}
	}()
	monomerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// We assume `inProcessConsensus` is true for now, so let's return an error if it's not.
	if !inProcessConsensus {
		return errors.New("in-process consensus must be enabled")
	}

	svrCfg, err := serverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		return fmt.Errorf("get server config: %v", err)
	}
	if err := svrCfg.ValidateBasic(); err != nil {
		return fmt.Errorf("validate server config: %v", err)
	}

	app, err := startApp(env, svrCtx, appCreator, opts)
	if err != nil {
		return fmt.Errorf("start application: %v", err)
	}

	engineURL, err := url.ParseString(svrCtx.Viper.GetString(flagEngineURL))
	if err != nil {
		return fmt.Errorf("parse engine url: %v", err)
	} else if scheme := engineURL.Scheme(); scheme != "ws" && scheme != "wss" {
		return fmt.Errorf("engine url needs to have scheme `ws` or `wss`, got %s", scheme)
	}

	monomerGenesisPath := svrCtx.Config.GenesisFile()
	appGenesis, err := genutiltypes.AppGenesisFromFile(monomerGenesisPath)
	if err != nil {
		return fmt.Errorf("load application genesis file: %v", err)
	}
	l2ChainID, err := strconv.ParseUint(appGenesis.ChainID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse chain ID: %v", err)
	}

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(
		env,
		svrCtx,
		&clientCtx,
		monomerCtx,
		app,
		opts,
		engineURL,
		l2ChainID,
		appGenesis.AppState,
		uint64(appGenesis.GenesisTime.Unix()),
	); err != nil {
		return fmt.Errorf("start Monomer node in-process: %v", err)
	}

	if svrCtx.Viper.GetBool(flagDev) {
		if err := startOPDevnet(monomerCtx, env, &cosmosToETHLogger{
			log: svrCtx.Logger,
		}, svrCtx.Viper, engineURL, l2ChainID); err != nil {
			return err
		}
	}

	<-sigCh

	return nil
}

func readFromFileOrGetDefault[T any](path string, getDefault func() (*T, error)) (*T, error) {
	if path == "" {
		structuredData, err := getDefault()
		if err != nil {
			return nil, fmt.Errorf("get default: %v", err)
		}
		return structuredData, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}
	var structuredData T
	if err := json.Unmarshal(data, &structuredData); err != nil {
		return nil, fmt.Errorf("unmarshal json: %v", err)
	}
	return &structuredData, nil
}

func startOPDevnet(
	ctx context.Context,
	env *environment.Env,
	logger ethlog.Logger,
	v *viper.Viper,
	engineURL *url.URL,
	l2ChainID uint64,
) error {
	// TODO: can we get the L2 genesis block from the genesis state in the config?
	engineClient, err := ethclient.DialContext(ctx, engineURL.String())
	if err != nil {
		return fmt.Errorf("dial engine: %v", err)
	}
	l2GenesisBlock, err := engineClient.BlockByNumber(ctx, new(big.Int).SetUint64(1))
	if err != nil {
		return fmt.Errorf("get l2 genesis block: %v", err)
	}

	l1Deployments, err := readFromFileOrGetDefault(v.GetString(flagL1DeploymentsPath), opdevnet.DefaultL1Deployments)
	if err != nil {
		return fmt.Errorf("get l1 deployments: %v", err)
	}
	deployConfig, err := readFromFileOrGetDefault(v.GetString(flagDeployConfigPath), func() (*opgenesis.DeployConfig, error) {
		return opdevnet.DefaultDeployConfig(l1Deployments)
	})
	if err != nil {
		return fmt.Errorf("get deploy config: %v", err)
	}
	deployConfig.L2ChainID = l2ChainID
	if time := l2GenesisBlock.Time(); time == 0 {
		// For some reason, Optimism's tooling (used in opdevnet.BuildL1Genesis) will convert a zero genesis timestamp into the current time.
		// This will break the time invariant for the L2 block: its L1 origin (the L1 genesis block) will have a more recent timestamp.
		// As a result, we disallow zero timestamps.
		// TODO: should this check happen in github.com/polymerdao/monomer/genesis.Commit?
		return errors.New("l2 genesis timestamp must be non-zero")
	} else {
		deployConfig.L1GenesisBlockTimestamp = hexutil.Uint64(time)
	}
	l1Allocs, err := readFromFileOrGetDefault(v.GetString(flagL1AllocsPath), opdevnet.DefaultL1Allocs)
	if err != nil {
		return fmt.Errorf("get l1 allocs: %v", err)
	}
	mneumonics, err := readFromFileOrGetDefault(v.GetString(flagMneumonicsPath), func() (*opdevnet.MnemonicConfig, error) {
		return opdevnet.DefaultMnemonicConfig, nil
	})
	if err != nil {
		return fmt.Errorf("get mneumonic config: %v", err)
	}
	secrets, err := mneumonics.Secrets()
	if err != nil {
		return fmt.Errorf("derive secrets from mneumonics: %v", err)
	}

	l1URL, err := url.ParseString(v.GetString(flagL1URL))
	if err != nil {
		return fmt.Errorf("parse l1 url: %v", err)
	}
	opNodeURL, err := url.ParseString(v.GetString(flagOPNodeURL))
	if err != nil {
		return fmt.Errorf("parse op node url: %v", err)
	}

	l1Config, err := opdevnet.BuildL1Config(deployConfig, l1Deployments, l1Allocs, l1URL, os.TempDir())
	if err != nil {
		return fmt.Errorf("build l1 config: %v", err)
	}
	if err := l1Config.Run(ctx, env, logger); err != nil {
		return fmt.Errorf("run l1: %v", err)
	}

	opConfig, err := opdevnet.BuildOPConfig(
		deployConfig,
		secrets.Batcher,
		secrets.Proposer,
		l1Config.Genesis.ToBlock(),
		l1Deployments.L2OutputOracleProxy,
		eth.HeaderBlockID(l2GenesisBlock.Header()),
		l1URL,
		opNodeURL,
		engineURL,
		engineURL,
		[32]byte{},
	)
	if err != nil {
		return fmt.Errorf("build op config: %v", err)
	}
	if err := opConfig.Run(ctx, env, logger); err != nil {
		return fmt.Errorf("run op: %v", err)
	}

	return nil
}

var _ io.WriteCloser = (*fakeTraceWriter)(nil)

type fakeTraceWriter struct{}

func (f *fakeTraceWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (f *fakeTraceWriter) Close() error {
	return nil
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L624
func startApp(
	env *environment.Env,
	svrCtx *server.Context,
	appCreator servertypes.AppCreator,
	opts server.StartCmdOptions,
) (servertypes.Application, error) {
	db, err := opts.DBOpener(svrCtx.Config.RootDir, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return nil, fmt.Errorf("open db: %v", err)
	}

	// TODO: Check if is testnet and implement `testnetify` function

	app := appCreator(svrCtx.Logger, db, &fakeTraceWriter{}, svrCtx.Viper)
	env.DeferErr("close app", app.Close)

	return app, nil
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L307
// We modify this function to start a Monomer node in-process instead of a Comet node.
func startInProcess(
	env *environment.Env,
	svrCtx *server.Context,
	clientCtx *client.Context,
	monomerCtx context.Context,
	app servertypes.Application,
	opts server.StartCmdOptions,
	engineWS *url.URL,
	l2ChainID uint64,
	appState json.RawMessage,
	genesisTime uint64,
) error {
	svrCtx.Logger.Info("Starting Monomer node in-process")
	if err := startMonomerNode(&WrappedApplication{
		app: app,
	}, env, monomerCtx, svrCtx, clientCtx, engineWS, l2ChainID, appState, genesisTime); err != nil {
		return fmt.Errorf("start Monomer node: %v", err)
	}

	if opts.PostSetup != nil {
		var g errgroup.Group
		if err := opts.PostSetup(svrCtx, *clientCtx, monomerCtx, &g); err != nil {
			return fmt.Errorf("run post setup: %v", err)
		}
		if err := g.Wait(); err != nil {
			return fmt.Errorf("unexpected error in PostSetup errgroup: %v", err)
		}
	}

	return nil
}

// Starts the Monomer node in-process in place of the Comet node. The
// `MonomerGenesisPath` flag must be set in viper before invoking this.
func startMonomerNode(
	wrappedApp *WrappedApplication,
	env *environment.Env,
	monomerCtx context.Context,
	svrCtx *server.Context,
	clientCtx *client.Context,
	engineURL *url.URL,
	l2ChainID uint64,
	appStateJSON json.RawMessage,
	genesisTime uint64,
) error {
	cmtListenAddr := svrCtx.Config.RPC.ListenAddress
	cmtListenAddr = strings.TrimPrefix(cmtListenAddr, "tcp://")
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return fmt.Errorf("create CometBFT listener: %v", err)
	}

	// Set the client and chain id on the client ctx so the dummy signer works.
	rpcclient, err := rpchttp.New(svrCtx.Config.RPC.ListenAddress, "/websocket")
	if err != nil {
		return err
	}
	*clientCtx = clientCtx.WithClient(rpcclient)
	clientCtx.ChainID = fmt.Sprintf("%d", l2ChainID)

	var blockPebbleDB *pebble.DB
	if backendType := dbm.BackendType(svrCtx.Config.DBBackend); backendType == dbm.MemDBBackend {
		blockPebbleDB, err = pebble.Open("", &pebble.Options{
			FS: vfs.NewMem(),
		})
	} else {
		if backendType != dbm.PebbleDBBackend {
			svrCtx.Logger.Info("Overriding provided db backend for the blockstore", "provided", backendType, "using", dbm.PebbleDBBackend)
		}
		blockPebbleDB, err = pebble.Open(filepath.Join(svrCtx.Config.RootDir, "blockstore"), nil)
	}
	if err != nil {
		return fmt.Errorf("open blockstore: %v", err)
	}
	env.DeferErr("close block db", blockPebbleDB.Close)

	txdb, err := cometdb.NewDB("tx", cometdb.BackendType(svrCtx.Config.DBBackend), svrCtx.Config.RootDir)
	if err != nil {
		return fmt.Errorf("create tx db: %v", err)
	}
	env.DeferErr("close tx db", txdb.Close)

	mempooldb, err := dbm.NewDB("mempool", dbm.BackendType(svrCtx.Config.DBBackend), svrCtx.Config.RootDir)
	if err != nil {
		return fmt.Errorf("create mempool db: %v", err)
	}
	env.DeferErr("close mempool db", mempooldb.Close)

	rawDB, err := rawdb.NewPebbleDBDatabase(
		svrCtx.Config.RootDir+"/ethstate",
		defaultCacheSize,
		defaultHandlesSize,
		"",
		false,
		false,
	)
	if err != nil {
		return fmt.Errorf("create raw db: %v", err)
	}
	env.DeferErr("close raw db", rawDB.Close)
	trieDB := triedb.NewDatabase(rawDB, nil)
	env.DeferErr("close trieDB", trieDB.Close)
	ethstatedb := state.NewDatabaseWithNodeDB(rawDB, trieDB)

	var appState map[string]json.RawMessage
	if err := json.Unmarshal(appStateJSON, &appState); err != nil {
		return fmt.Errorf("unmarshal app state: %v", err)
	}

	engineWS, err := net.Listen("tcp", engineURL.Host())
	if err != nil {
		return fmt.Errorf("create engine listener: %v", err)
	}
	n := node.New(
		wrappedApp,
		clientCtx,
		&genesis.Genesis{
			ChainID:  monomer.ChainID(l2ChainID),
			AppState: appState,
			Time:     genesisTime,
		},
		engineWS,
		cometListener,
		localdb.New(blockPebbleDB),
		mempooldb,
		txdb,
		ethstatedb,
		svrCtx.Config.Instrumentation,
		&node.SelectiveListener{
			OnEngineHTTPServeErrCb: func(err error) {
				svrCtx.Logger.Error("[Engine HTTP Server]", "error", err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				svrCtx.Logger.Error("[Engine Websocket]", "error", err)
			},
			OnCometServeErrCb: func(err error) {
				svrCtx.Logger.Error("[CometBFT]", "error", err)
			},
			OnPrometheusServeErrCb: func(err error) {
				svrCtx.Logger.Error("[Prometheus]", "error", err)
			},
		},
	)
	svrCtx.Logger.Info("Spinning up Monomer node")

	if err := n.Run(monomerCtx, env); err != nil {
		return fmt.Errorf("run Monomer node: %v", err)
	}

	svrCtx.Logger.Info("Monomer started w/ CometBFT listener on", "address", cometListener.Addr())

	return nil
}
