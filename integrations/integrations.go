package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	"github.com/spf13/viper"
)

func StartCommandHandler(
	svrCtx *server.Context,
	clientCtx client.Context, //nolint:gocritic // hugeParam
	appCreator servertypes.AppCreator,
	inProcessConsesus bool,
	opts server.StartCmdOptions,
) error {
	// We assume `inProcessConsensus` is true for now, so let's return an error if it's not.
	if !inProcessConsesus {
		return fmt.Errorf("in-process consensus must be enabled")
	}

	env := environment.New()
	defer func() {
		if err := env.Close(); err != nil {
			svrCtx.Logger.Error("Failed to close environment", "err", err)
		}
	}()

	svrCfg, err := serverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}
	if err := svrCfg.ValidateBasic(); err != nil {
		return fmt.Errorf("failed to validate server config: %w", err)
	}

	app, err := startApp(env, svrCtx, appCreator, opts)
	if err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(env, svrCtx, clientCtx, app, opts); err != nil {
		svrCtx.Logger.Error("failed to start Monomer node in-process", "error", err)
		return fmt.Errorf("failed to start Monomer node in-process: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

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
) (app servertypes.Application, err error) {
	db, err := opts.DBOpener(svrCtx.Config.RootDir, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return app, fmt.Errorf("open db: %v", err)
	}

	// TODO: Check if is testnet and implement `testnetify` function

	app = appCreator(svrCtx.Logger, db, &fakeTraceWriter{}, svrCtx.Viper)
	env.Defer(func() {
		if localErr := app.Close(); localErr != nil {
			svrCtx.Logger.Error(localErr.Error())
		}
	})

	return app, nil
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L307
// We modify this function to start a Monomer node in-process instead of a Comet node.
func startInProcess(
	env *environment.Env,
	svrCtx *server.Context,
	clientCtx client.Context, //nolint:gocritic // hugeParam
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	g, ctx := getCtx(svrCtx, true)

	svrCtx.Logger.Info("Starting Monomer node in-process")
	wrappedApp := &WrappedApplication{app}
	_, err := startMonomerNode(wrappedApp, env, svrCtx)
	if err != nil {
		return fmt.Errorf("failed to start Monomer node: %v", err)
	}

	if opts.PostSetup != nil {
		if err := opts.PostSetup(svrCtx, clientCtx, ctx, g); err != nil {
			return fmt.Errorf("failed to run post setup: %v", err)
		}
	}

	return g.Wait()
}

// Starts the Monomer node in-process in place of the Comet node. The
// `MonomerGenesisPath` flag must be set in viper before invoking this.
func startMonomerNode(wrappedApp *WrappedApplication, env *environment.Env, svrCtx *server.Context) (*node.Node, error) {
	chainID := monomer.ChainID(1)
	engineWS, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("create Engine websocket listener: %v", err)
	}

	cmtListenAddr := svrCtx.Config.RPC.ListenAddress
	cmtListenAddr = strings.TrimPrefix(cmtListenAddr, "tcp://")
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return nil, fmt.Errorf("create CometBFT listener: %v", err)
	}

	appdb := dbm.NewMemDB()
	env.DeferErr("close app db", appdb.Close)
	blockdb := dbm.NewMemDB()
	env.DeferErr("close block db", blockdb.Close)
	txdb := cometdb.NewMemDB()
	env.DeferErr("close tx db", txdb.Close)
	mempooldb := dbm.NewMemDB()
	env.DeferErr("close mempool db", mempooldb.Close)

	monomerGenesisPath := viper.GetString("monomer-genesis-path")
	svrCtx.Logger.Info("Loading Monomer genesis from", "path", monomerGenesisPath)

	appGenesis, err := genutiltypes.AppGenesisFromFile(monomerGenesisPath)
	if err != nil {
		return nil, fmt.Errorf("load application genesis file: %v", err)
	}

	var appState map[string]json.RawMessage
	if err := json.Unmarshal(appGenesis.AppState, &appState); err != nil {
		svrCtx.Logger.Error("Failed to unmarshal app state", "error", err)
		return nil, fmt.Errorf("failed to unmarshal app state: %v", err)
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
			OnEngineHTTPServeErrCb: func(err error) {
				svrCtx.Logger.Error("[Engine HTTP Server]", "error", err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				svrCtx.Logger.Error("[Engine Websocket]", "error", err)
			},
			OnCometServeErrCb: func(err error) {
				svrCtx.Logger.Error("[CometBFT]", "error", err)
			},
		},
	)
	svrCtx.Logger.Info("Spinning up Monomer node")

	nodeCtx, nodeCtxCancel := context.WithCancel(context.Background())
	env.Defer(nodeCtxCancel)

	nodeErr := n.Run(nodeCtx, env)
	if nodeErr != nil {
		return nil, fmt.Errorf("failed to run Monomer node: %v", nodeErr)
	}

	svrCtx.Logger.Info("Monomer started w/ CometBFT listener on", "address", cometListener.Addr())

	env.Defer(func() {
		engineWS.Close()
		cometListener.Close()
	})

	return n, nil
}
