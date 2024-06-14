package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
)

func StartCommandHandler(
	svrCtx *server.Context,
	clientCtx client.Context,
	appCreator servertypes.AppCreator,
	inProcessConsesus bool,
	opts server.StartCmdOptions,
) error {
	// We assume `inProcessConsensus` is true for now, so let's return an error if it's not.
	if !inProcessConsesus {
		return fmt.Errorf("in-process consensus must be enabled")
	}

	// --- Do the normal Cosmos SDK stuff ---
	svrCfg, err := GetAndValidateConfig(svrCtx)
	if err != nil {
		return fmt.Errorf("failed to get and validate server config: %w", err)
	}

	app, appCleanupFn, err := startApp(svrCtx, appCreator, opts)
	if err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}
	defer appCleanupFn()

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	return startInProcess(svrCtx, svrCfg, clientCtx, app, opts)
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L624
func startApp(
	svrCtx *server.Context,
	appCreator servertypes.AppCreator,
	opts server.StartCmdOptions,
) (app servertypes.Application, cleanupFn func(), err error) {
	traceWriter, traceCleanupFn, err := SetupTraceWriter(svrCtx.Logger, svrCtx.Viper.GetString(FlagTraceStore))
	if err != nil {
		return app, cleanupFn, err
	}

	home := svrCtx.Config.RootDir
	db, err := opts.DBOpener(home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return app, cleanupFn, err
	}

	// TODO: Check if is testnet and implement `testnetify` function

	app = appCreator(svrCtx.Logger, db, traceWriter, svrCtx.Viper)
	cleanupFn = func() {
		traceCleanupFn()
		if localErr := app.Close(); localErr != nil {
			svrCtx.Logger.Error(localErr.Error())
		}
	}

	return app, cleanupFn, nil
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L307
// We modify this function to start a Monomer node in-process instead of a Comet node.
func startInProcess(
	svrCtx *server.Context,
	svrCfg serverconfig.Config,
	clientCtx client.Context,
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	cmtCfg := svrCtx.Config
	gRPCOnly := svrCtx.Viper.GetBool(FlagGRPCOnly)

	g, ctx := GetCtx(svrCtx, true)

	if gRPCOnly {
		svrCtx.Logger.Info("starting node in gRPC only mode; Monomer is disabled")
		svrCfg.GRPC.Enable = true
	} else {
		svrCtx.Logger.Info("starting Monomer node in-process")
		// Start the Monomer node
		monomerEnv := environment.New()
		wrappedApp := &WrappedApplication{app}
		_, err := startMonomerNode(wrappedApp, monomerEnv, svrCtx)
		if err != nil {
			return err
		}

		// Add the tx service to the gRPC router. We only need to register this
		// service if API or gRPC is enabled, and avoid doing so in the general
		// case, because it spawns a new local CometBFT RPC client.
		/* if svrCfg.API.Enable || svrCfg.GRPC.Enable {
		    clientCtx = clientCtx.WithClient(local.New(tmNode))
		    app.RegisterTxService(clientCtx)
		    app.RegisterTendermintService(clientCtx)
		    app.RegisterNodeService(clientCtx, svrCfg)
		} */
	}

	grpcSrv, clientCtx, err := StartGrpcServer(ctx, g, svrCfg.GRPC, clientCtx, svrCtx, app)
	if err != nil {
		return err
	}

	err = StartAPIServer(ctx, g, svrCfg, clientCtx, svrCtx, app, cmtCfg.RootDir, grpcSrv)
	if err != nil {
		return err
	}

	if opts.PostSetup != nil {
		if err := opts.PostSetup(svrCtx, clientCtx, ctx, g); err != nil {
			return err
		}
	}

	return g.Wait()
}

// Starts the Monomer node in-process in place of the Comet node.
func startMonomerNode(wrappedApp *WrappedApplication, env *environment.Env, svrCtx *server.Context) (*node.Node, error) {
	chainID := monomer.ChainID(1)
	engineWS, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	cmtListenAddr := svrCtx.Config.P2P.ListenAddress
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return nil, err
	}

	appdb := dbm.NewMemDB()
	blockdb := dbm.NewMemDB()
	txdb := cometdb.NewMemDB()
	mempooldb := dbm.NewMemDB()

	appGenesis, err := genutiltypes.AppGenesisFromFile(svrCtx.Config.GenesisFile())
	if err != nil {
		return nil, err
	}

	var appState map[string]json.RawMessage
	if err := json.Unmarshal(appGenesis.AppState, &appState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal app state: %w", err)
	}

	n := node.New(
		wrappedApp,
		&genesis.Genesis{
			ChainID:  chainID,
			AppState: appState,
		},
		engineWS,
		cometListener,
		blockdb,
		mempooldb,
		txdb,
		&node.SelectiveListener{
			// TODO: Remove this field from SelectiveListener
			OnEngineHTTPServeErrCb: func(err error) {
				fmt.Errorf("failed to serve engine HTTP: %w", err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				fmt.Errorf("failed to serve engine websocket: %w", err)
			},
		},
	)

	nodeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env.Go(func() {
		n.Run(nodeCtx, env)
	})
	env.Defer(func() {
		engineWS.Close()
		cometListener.Close()
		appdb.Close()
		blockdb.Close()
		txdb.Close()
		mempooldb.Close()
	})

	env.Close()

	return n, nil
}
