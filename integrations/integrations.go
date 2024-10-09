package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/utils"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	monomerEngineWSFlag = "monomer-engine-ws"
	defaultCacheSize    = 16 // 16 MB
	defaultHandlesSize  = 16
)

var sigCh = make(chan os.Signal, 1)

// StartCommandHandler is a custom callback that overrides the default `start` function in the Cosmos
// SDK. It starts a Monomer node in-process instead of a CometBFT node.
func StartCommandHandler(
	svrCtx *server.Context,
	clientCtx client.Context, //nolint:gocritic // hugeParam
	appCreator servertypes.AppCreator,
	inProcessConsensus bool,
	opts server.StartCmdOptions,
) error {
	// We assume `inProcessConsensus` is true for now, so let's return an error if it's not.
	if !inProcessConsensus {
		return errors.New("in-process consensus must be enabled")
	}

	env := environment.New()
	defer func() {
		if err := env.Close(); err != nil {
			svrCtx.Logger.Error("Failed to close environment", "err", err)
		}
	}()

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

	g, monomerCtx := getCtx(svrCtx)

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err = startInProcess(env, g, svrCtx, &svrCfg, &clientCtx, monomerCtx, app, opts); err != nil {
		return fmt.Errorf("start Monomer node in-process: %v", err)
	}

	if err = g.Wait(); err != nil {
		return fmt.Errorf("unexpected error in errgroup: %v", err)
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
	g *errgroup.Group,
	svrCtx *server.Context,
	svrCfg *serverconfig.Config,
	clientCtx *client.Context,
	monomerCtx context.Context,
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	svrCtx.Logger.Info("Starting Monomer node in-process")
	err := startMonomerNode(&WrappedApplication{
		app: app,
	}, env, monomerCtx, svrCtx, clientCtx)
	if err != nil {
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
		grpcSrv, clientCtx, err = startGrpcServer(monomerCtx, g, svrCfg.GRPC, clientCtx, svrCtx, app)
		if err != nil {
			return fmt.Errorf("start gRPC server: %v", err)
		}
	}

	if svrCfg.API.Enable {
		startAPIServer(monomerCtx, g, svrCfg, clientCtx, svrCtx, app, svrCtx.Config.RootDir, grpcSrv)
	}

	if opts.PostSetup != nil {
		if err = opts.PostSetup(svrCtx, *clientCtx, monomerCtx, g); err != nil {
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
) error {
	engineWS, err := net.Listen("tcp", viper.GetString(monomerEngineWSFlag))
	if err != nil {
		return fmt.Errorf("create Engine websocket listener: %v", err)
	}

	cmtListenAddr := svrCtx.Config.RPC.ListenAddress
	cmtListenAddr = strings.TrimPrefix(cmtListenAddr, "tcp://")
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return fmt.Errorf("create CometBFT listener: %v", err)
	}

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

	monomerGenesisPath := svrCtx.Config.GenesisFile()

	appGenesis, err := genutiltypes.AppGenesisFromFile(monomerGenesisPath)
	if err != nil {
		return fmt.Errorf("load application genesis file: %v", err)
	}

	genChainID, err := strconv.ParseUint(appGenesis.ChainID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse chain ID: %v", err)
	}

	var appState map[string]json.RawMessage
	if err := json.Unmarshal(appGenesis.AppState, &appState); err != nil {
		return fmt.Errorf("unmarshal app state: %v", err)
	}

	n := node.New(
		wrappedApp,
		clientCtx,
		&genesis.Genesis{
			ChainID:  monomer.ChainID(genChainID),
			AppState: appState,
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
