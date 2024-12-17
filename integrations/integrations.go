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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethdb"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/polymerdao/monomer/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	flagEngineURL         = "monomer.engine-url"
	flagSequencerMode     = "monomer.sequencer"
	flagDev               = "monomer.dev-start"
	flagL1AllocsPath      = "monomer.dev.l1-allocs"
	flagL1DeploymentsPath = "monomer.dev.l1-deployments"
	flagDeployConfigPath  = "monomer.dev.deploy-config"
	flagMneumonicsPath    = "monomer.dev.mneumonics"
	flagL1URL             = "monomer.dev.l1-url"
	flagOPNodeURL         = "monomer.dev.op-node-url"
	flagL1UserAddress     = "monomer.dev.l1-user-address"

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
			cmd.Flags().Bool(flagSequencerMode, false, "enable sequencer mode for the Monomer node")
			cmd.Flags().Bool(flagDev, false, "run the OP Stack devnet in-process for testing")
			cmd.Flags().String(flagL1URL, "ws://127.0.0.1:9001", "")
			cmd.Flags().String(flagOPNodeURL, "http://127.0.0.1:9002", "")
			cmd.Flags().String(flagL1DeploymentsPath, "", "")
			cmd.Flags().String(flagDeployConfigPath, "", "")
			cmd.Flags().String(flagL1AllocsPath, "", "")
			cmd.Flags().String(flagMneumonicsPath, "", "")
			cmd.Flags().String(flagL1UserAddress, "", "address of the user's L1 account to mint ETH on genesis to")
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

	isDevnet := svrCtx.Viper.GetBool(flagDev)

	app, err := startApp(env, svrCtx, appCreator, isDevnet, opts)
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

	g, monomerCtx := getCtx(svrCtx)
	env.DeferErr("unexpected error in errgroup", g.Wait)

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(
		env,
		g,
		svrCtx,
		&svrCfg,
		&clientCtx,
		monomerCtx,
		app,
		opts,
		engineURL,
		l2ChainID,
		appGenesis.AppState,
		uint64(appGenesis.GenesisTime.Unix()),
		isDevnet,
	); err != nil {
		return fmt.Errorf("start Monomer node in-process: %v", err)
	}

	if isDevnet {
		if err := startOPDevnet(monomerCtx, env, &cosmosToETHLogger{
			log: svrCtx.Logger,
		}, svrCtx.Viper, engineURL, l2ChainID); err != nil {
			return err
		}
	}

	if err = g.Wait(); err != nil {
		return fmt.Errorf("unexpected error in errgroup: %v", err)
	}

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
	// TODO thinking about removing this clunky readFromFileOrGetDefault abstraction.
	l1AllocsForge, err := readFromFileOrGetDefault(v.GetString(flagL1AllocsPath), func() (*opgenesis.ForgeDump, error) {
		got, err := opdevnet.DefaultL1Allocs()
		if err != nil {
			return nil, err
		}
		gotForge := opgenesis.ForgeDump(*got)
		return &gotForge, nil
	})
	if err != nil {
		return fmt.Errorf("get l1 allocs: %v", err)
	}
	l1Allocs := state.Dump(*l1AllocsForge)
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

	if l1UserAddress := v.GetString(flagL1UserAddress); l1UserAddress != "" {
		l1Allocs.Accounts[l1UserAddress] = state.DumpAccount{
			Balance: "0x152D02C7E14AF6800000", // 100,000 ETH
			Nonce:   0,
		}
	}

	l1Config, err := opdevnet.BuildL1Config(deployConfig, l1Deployments, &l1Allocs, l1URL, os.TempDir())
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
	opConfig.Node.Driver.SequencerEnabled = v.GetBool(flagSequencerMode)
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
	devnet bool,
	opts server.StartCmdOptions,
) (servertypes.Application, error) {
	if devnet {
		svrCtx.Viper.Set("app-db-backend", string(dbm.MemDBBackend))
	}
	backendType := server.GetAppDBBackend(svrCtx.Viper)
	db, err := opts.DBOpener(svrCtx.Config.RootDir, backendType)
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
	g *errgroup.Group,
	svrCtx *server.Context,
	svrCfg *serverconfig.Config,
	clientCtx *client.Context,
	monomerCtx context.Context,
	app servertypes.Application,
	opts server.StartCmdOptions,
	engineWS *url.URL,
	l2ChainID uint64,
	appState json.RawMessage,
	genesisTime uint64,
	isDevnet bool,
) error {
	svrCtx.Logger.Info("Starting Monomer node in-process")
	if err := startMonomerNode(&WrappedApplication{
		app: app,
	}, env, monomerCtx, svrCtx, clientCtx, engineWS, l2ChainID, appState, genesisTime, isDevnet); err != nil {
		return fmt.Errorf("start Monomer node: %v", err)
	}

	// Add the tx service to the gRPC router if API or gRPC is enabled.
	if svrCfg.API.Enable || svrCfg.GRPC.Enable {
		app.RegisterTxService(*clientCtx)
		app.RegisterTendermintService(*clientCtx)
		app.RegisterNodeService(*clientCtx, *svrCfg)
	}

	var grpcSrv *grpc.Server
	if svrCfg.GRPC.Enable {
		var err error
		grpcSrv, clientCtx, err = startGrpcServer(monomerCtx, g, svrCfg.GRPC, clientCtx, svrCtx, app)
		if err != nil {
			return fmt.Errorf("start gRPC server: %v", err)
		}
	}

	if svrCfg.API.Enable {
		startAPIServer(monomerCtx, g, svrCfg, clientCtx, svrCtx, app, svrCtx.Config.RootDir, grpcSrv)
	}

	if opts.PostSetup != nil {
		if err := opts.PostSetup(svrCtx, *clientCtx, monomerCtx, g); err != nil {
			return fmt.Errorf("run post setup: %v", err)
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
	devnet bool,
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

	backendType := dbm.BackendType(svrCtx.Config.DBBackend)
	if devnet {
		backendType = dbm.MemDBBackend
		svrCtx.Logger.Info("Overriding provided db backend to use in-memory for devnet")
	}

	var blockPebbleDB *pebble.DB
	if backendType == dbm.MemDBBackend {
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

	txdb, err := cometdb.NewDB("tx", cometdb.BackendType(backendType), svrCtx.Config.RootDir)
	if err != nil {
		return fmt.Errorf("create tx db: %v", err)
	}
	env.DeferErr("close tx db", txdb.Close)

	mempooldb, err := dbm.NewDB("mempool", backendType, svrCtx.Config.RootDir)
	if err != nil {
		return fmt.Errorf("create mempool db: %v", err)
	}
	env.DeferErr("close mempool db", mempooldb.Close)

	var rawDB ethdb.Database
	if backendType == dbm.MemDBBackend {
		rawDB = rawdb.NewMemoryDatabase()
	} else {
		rawDB, err = rawdb.NewPebbleDBDatabase(
			filepath.Join(svrCtx.Config.RootDir, "ethstate"),
			defaultCacheSize,
			defaultHandlesSize,
			"",
			false,
			false,
		)
		if err != nil {
			return fmt.Errorf("create raw db: %v", err)
		}
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

// Starts the gRPC server if enabled in the server configuration.
func startGrpcServer(
	monomerCtx context.Context,
	g *errgroup.Group,
	config serverconfig.GRPCConfig,
	clientCtx *client.Context,
	svrCtx *server.Context,
	app servertypes.Application,
) (*grpc.Server, *client.Context, error) {
	_, _, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, clientCtx, fmt.Errorf("invalid gRPC server address: %v", err)
	}

	maxSendMsgSize := config.MaxSendMsgSize
	if maxSendMsgSize == 0 {
		maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
	}

	maxRecvMsgSize := config.MaxRecvMsgSize
	if maxRecvMsgSize == 0 {
		maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
	}

	// if gRPC is enabled, configure gRPC client for gRPC gateway
	grpcClient, err := grpc.NewClient(
		config.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
			grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(maxSendMsgSize),
		),
	)
	if err != nil {
		return nil, clientCtx, fmt.Errorf("failed to dial gRPC server: %v", err)
	}

	clientCtx = utils.Ptr(clientCtx.WithGRPCClient(grpcClient))

	grpcSrv, err := servergrpc.NewGRPCServer(*clientCtx, app, config)
	if err != nil {
		return nil, clientCtx, fmt.Errorf("failed to create gRPC server: %v", err)
	}

	// Start the gRPC server in a goroutine. Note, the provided ctx will ensure
	// that the server is gracefully shut down.
	g.Go(func() error {
		return servergrpc.StartGRPCServer(monomerCtx, svrCtx.Logger.With("module", "grpc-server"), config, grpcSrv)
	})
	return grpcSrv, clientCtx, nil
}

// Starts the API server if enabled in the server configuration.
func startAPIServer(
	monomerCtx context.Context,
	g *errgroup.Group,
	svrCfg *serverconfig.Config,
	clientCtx *client.Context,
	svrCtx *server.Context,
	app servertypes.Application,
	home string,
	grpcSrv *grpc.Server,
) {
	clientCtx = utils.Ptr(clientCtx.WithHomeDir(home))

	apiSrv := api.New(*clientCtx, svrCtx.Logger.With("module", "api-server"), grpcSrv)
	app.RegisterAPIRoutes(apiSrv, svrCfg.API)

	// Start the API server in a goroutine. Note, the provided ctx will ensure
	// that the server is gracefully shut down.
	g.Go(func() error {
		return apiSrv.Start(monomerCtx, *svrCfg)
	})
}

// getCtx returns a new errgroup and context for the Monomer node.
func getCtx(svrCtx *server.Context) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	// Listen for SIGINT and SIGTERM quit signals. When a signal is received,
	// the cleanup function is called, indicating the caller can gracefully exit or
	// return.
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	g.Go(func() error {
		sig := <-sigCh
		svrCtx.Logger.Info("Shutting down Monomer...", "signal", sig.String())
		cancelFn()
		return nil
	})

	return g, ctx
}
