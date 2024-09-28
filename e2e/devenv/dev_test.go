package devenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cometbft/cometbft/config"
	"github.com/ethereum/go-ethereum/log"
	"github.com/polymerdao/monomer/e2e"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/node"
	"github.com/stretchr/testify/require"

	"golang.org/x/exp/slog"
)

func openLogFile(t *testing.T, env *environment.Env, name string) *os.File {
	filename := filepath.Join("artifacts", name+".log")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	env.DeferErr("close log file: "+filename, file.Close)
	return file
}

func TestDevEnv(t *testing.T) {
	t.Log("\n\n\nRunning e2e stack in dev mode\n\nUse cmd+C / ctrl+C to stop the stack\n\n")

	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()

	if err := os.Mkdir("artifacts", 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}

	// Unfortunately, geth and parts of the OP Stack occasionally use the root logger.
	// We capture the root logger's output in a separate file.
	log.SetDefault(log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "root-logger"), false)))

	opLogger := log.NewTerminalHandler(openLogFile(t, env, "op"), false)

	prometheusCfg := &config.InstrumentationConfig{
		Prometheus: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := e2e.Setup(ctx, env, prometheusCfg, &e2e.SelectiveListener{
		OPLogCb: func(r slog.Record) {
			require.NoError(t, opLogger.Handle(context.Background(), r))
		},
		NodeSelectiveListener: &node.SelectiveListener{
			OnEngineHTTPServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnCometServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnPrometheusServeErrCb: func(err error) {
				require.NoError(t, err)
			},
		},
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Hour) // four day testnet
}
