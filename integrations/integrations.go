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
	"strconv"
	"strings"
	"syscall"

	cometdb "github.com/cometbft/cometbft-db"
	cometconfig "github.com/cometbft/cometbft/config"
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

const (
	monomerGenesisPathFlag = "monomer-genesis-path"
	monomerEngineWSFlag    = "monomer-engine-ws"
)

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

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(env, svrCtx, &clientCtx, app, opts); err != nil {
		return fmt.Errorf("start Monomer node in-process: %v", err)
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
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	g, ctx := getCtx(svrCtx, true)

	svrCtx.Logger.Info("Starting Monomer node in-process")
	err := startMonomerNode(&WrappedApplication{
		app: app,
	}, env, svrCtx)
	if err != nil {
		return fmt.Errorf("start Monomer node: %v", err)
	}

	if opts.PostSetup != nil {
		if err := opts.PostSetup(svrCtx, *clientCtx, ctx, g); err != nil {
			return fmt.Errorf("run post setup: %v", err)
		}
	}

	return g.Wait()
}

// Starts the Monomer node in-process in place of the Comet node. The
// `MonomerGenesisPath` flag must be set in viper before invoking this.
func startMonomerNode(wrappedApp *WrappedApplication, env *environment.Env, svrCtx *server.Context) error {
	engineWS, err := net.Listen("tcp", viper.GetString(monomerEngineWSFlag))
	if err != nil {
		return fmt.Errorf("create Engine websocket listener: %v", err)
	}
	env.DeferErr("close engine ws", engineWS.Close)

	cmtListenAddr := svrCtx.Config.RPC.ListenAddress
	cmtListenAddr = strings.TrimPrefix(cmtListenAddr, "tcp://")
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return fmt.Errorf("create CometBFT listener: %v", err)
	}
	env.DeferErr("close comet listener", cometListener.Close)

	appdb, err := cometconfig.DefaultDBProvider(&cometconfig.DBContext{
		ID:     "app",
		Config: svrCtx.Config,
	})
	if err != nil {
		return fmt.Errorf("create app db: %v", err)
	}
	env.DeferErr("close app db", appdb.Close)
	blockdb := dbm.NewMemDB()
	env.DeferErr("close block db", blockdb.Close)
	txdb := cometdb.NewMemDB()
	env.DeferErr("close tx db", txdb.Close)
	mempooldb := dbm.NewMemDB()
	env.DeferErr("close mempool db", mempooldb.Close)

	monomerGenesisPath := viper.GetString(monomerGenesisPathFlag)
	svrCtx.Logger.Info("Loading Monomer genesis from", "path", monomerGenesisPath)

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
		&genesis.Genesis{
			ChainID:  monomer.ChainID(genChainID),
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

	if err := n.Run(nodeCtx, env); err != nil {
		return fmt.Errorf("run Monomer node: %v", err)
	}

	svrCtx.Logger.Info("Monomer started w/ CometBFT listener on", "address", cometListener.Addr())

	return nil
}
