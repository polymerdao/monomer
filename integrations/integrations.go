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
		if closeErr := env.Close(); closeErr != nil {
			svrCtx.Logger.Error(closeErr.Error())
		}
	}()

	// --- Do the normal Cosmos SDK stuff ---
	svrCfg, err := getAndValidateConfig(svrCtx)
	if err != nil {
		return fmt.Errorf("failed to get and validate server config: %w", err)
	}

	app, err := startApp(env, svrCtx, appCreator, opts)
	if err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(env, svrCtx, svrCfg, clientCtx, app, opts); err != nil {
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
	home := svrCtx.Config.RootDir
	db, err := opts.DBOpener(home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return app, err
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
	_ serverconfig.Config, //nolint:gocritic // hugeParam
	clientCtx client.Context, //nolint:gocritic // hugeParam
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	g, ctx := getCtx(svrCtx, true)

	svrCtx.Logger.Info("starting Monomer node in-process")
	// --- Start the Monomer node ---
	wrappedApp := &WrappedApplication{app}
	_, err := startMonomerNode(wrappedApp, env, svrCtx)
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

// Starts the Monomer node in-process in place of the Comet node. The
// `MonomerGenesisPath` flag must be set in viper before invoking this.
func startMonomerNode(wrappedApp *WrappedApplication, env *environment.Env, svrCtx *server.Context) (*node.Node, error) {
	// --- Bootstrap listeners and DBs ---
	chainID := monomer.ChainID(1)
	engineWS, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	cmtListenAddr := svrCtx.Config.RPC.ListenAddress
	cmtListenAddr = strings.TrimPrefix(cmtListenAddr, "tcp://")
	cometListener, err := net.Listen("tcp", cmtListenAddr)
	if err != nil {
		return nil, err
	}

	appdb := dbm.NewMemDB()
	blockdb := dbm.NewMemDB()
	txdb := cometdb.NewMemDB()
	mempooldb := dbm.NewMemDB()

	// --- Load Monomer genesis doc ---
	monomerGenesisPath := viper.GetString("monomer-genesis-path")
	svrCtx.Logger.Info("loading Monomer genesis from", "path", monomerGenesisPath)

	appGenesis, err := genutiltypes.AppGenesisFromFile(monomerGenesisPath)
	if err != nil {
		svrCtx.Logger.Error("failed to load Monomer genesis", "error", err)
		return nil, err
	}

	var appState map[string]json.RawMessage
	if err := json.Unmarshal(appGenesis.AppState, &appState); err != nil {
		svrCtx.Logger.Error("failed to unmarshal app state", "error", err)
		return nil, fmt.Errorf("failed to unmarshal app state: %w", err)
	}

	// --- Create a Monomer node ---
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
				svrCtx.Logger.Error("failed to serve engine HTTP", "error", err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				svrCtx.Logger.Error("failed to serve engine websocket", "error", err)
			},
			OnCometServeErrCb: func(err error) {
				svrCtx.Logger.Error("failed to serve CometBFT", "error", err)
			},
		},
	)
	svrCtx.Logger.Info("spinning up Monomer node")

	nodeCtx, nodeCtxCancel := context.WithCancel(context.Background())

	// --- Run Monomer ---
	nodeErr := n.Run(nodeCtx, env)
	if nodeErr != nil {
		svrCtx.Logger.Error("failed to run Monomer node", "error", err)
	}

	// --- Check if CometBFT listener is up ---
	if conn, err := net.Dial("tcp", cmtListenAddr); err != nil {
		svrCtx.Logger.Error("failed to dial CometBFT", "error", err)
	} else {
		svrCtx.Logger.Info("dialing CometBFT succeeded at", "address", cmtListenAddr)
		conn.Close()
	}

	svrCtx.Logger.Info("Monomer started w/ CometBFT listener on", "address", cometListener.Addr())

	// --- Cleanup ---
	env.Defer(func() {
		engineWS.Close()
		cometListener.Close()
		appdb.Close()
		blockdb.Close()
		txdb.Close()
		mempooldb.Close()
		nodeCtxCancel()
	})

	return n, nil
}
