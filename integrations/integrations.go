package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

const (
	monomerEngineWSFlag = "monomer-engine-ws"
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

	monomerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Would usually start a Comet node in-process here, but we replace the
	// Comet node with a Monomer node.
	if err := startInProcess(env, svrCtx, &clientCtx, monomerCtx, app, opts); err != nil {
		return fmt.Errorf("start Monomer node in-process: %v", err)
	}

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
	monomerCtx context.Context,
	app servertypes.Application,
	opts server.StartCmdOptions,
) error {
	svrCtx.Logger.Info("Starting Monomer node in-process")
	err := startMonomerNode(&WrappedApplication{
		app: app,
	}, env, monomerCtx, svrCtx)
	if err != nil {
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

	blockdb, err := dbm.NewDB("blockstore", dbm.BackendType(svrCtx.Config.DBBackend), svrCtx.Config.RootDir)
	if err != nil {
		return fmt.Errorf("create block db: %v", err)
	}
	env.DeferErr("close block db", blockdb.Close)

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

	rawstatedb := rawdb.NewMemoryDatabase()
	ethstatedb, err := state.New(types.EmptyRootHash, state.NewDatabase(rawstatedb), nil)
	env.DeferErr("close eth state db", rawstatedb.Close)

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
		&genesis.Genesis{
			ChainID:  monomer.ChainID(genChainID),
			AppState: appState,
		},
		engineWS,
		cometListener,
		blockdb,
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
