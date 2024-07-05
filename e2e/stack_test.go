package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

const artifactsDirectoryName = "artifacts"

func openLogFile(t *testing.T, env *environment.Env, name string) *os.File {
	filename := filepath.Join(artifactsDirectoryName, name+".log")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	env.DeferErr("close log file: "+filename, file.Close)
	return file
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	const l1BlockTime = uint64(1) // We would want 250ms instead, but blocktimes are uints in seconds.

	deployConfigDir, err := filepath.Abs("./optimism/packages/contracts-bedrock/deploy-config")
	require.NoError(t, err)

	l1URL := newURL(t, "ws://127.0.0.1:8545")
	monomerEngineURL := newURL(t, "ws://127.0.0.1:8889")
	monomerCometURL := newURL(t, "http://127.0.0.1:8890")
	opNodeURL := newURL(t, "http://127.0.0.1:8891")

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()

	if err := os.Mkdir(artifactsDirectoryName, 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}
	opLogger := log.NewTerminalHandler(openLogFile(t, env, "op"), false)

	stack := e2e.New(l1URL, monomerEngineURL, monomerCometURL, opNodeURL, deployConfigDir, l1BlockTime, &e2e.SelectiveListener{
		OPLogCb: func(r slog.Record) {
			require.NoError(t, opLogger.Handle(context.Background(), r))
		},
		HandleCmdOutputCb: func(path string, stdout, stderr io.Reader) {
			file := openLogFile(t, env, filepath.Base(path))
			env.Go(func() {
				_, err := io.Copy(file, io.MultiReader(stdout, stderr))
				require.NoError(t, err)
			})
		},
		OnAnvilErrCb: func(err error) {
			t.Log(err)
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
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stack.Run(ctx, env))
	// To avoid flaky tests, hang until the Monomer server is ready.
	// We rely on the `go test` timeout to ensure the tests don't hang forever (default is 10 minutes).
	require.True(t, monomerEngineURL.IsReachable(ctx))
	monomerRPCClient, err := rpc.DialContext(ctx, monomerEngineURL.String())
	require.NoError(t, err)
	monomerClient := e2e.NewMonomerClient(monomerRPCClient)

	const targetHeight = 5

	client, err := bftclient.New(monomerCometURL.String(), monomerCometURL.String())
	require.NoError(t, err, "create Comet client")

	txBytes := testapp.ToTx(t, "userTxKey", "userTxValue")
	bftTx := bfttypes.Tx(txBytes)

	putTx, err := client.BroadcastTxAsync(ctx, txBytes)
	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, putTx.Code, "put.Code is not OK")
	require.EqualValues(t, bftTx.Hash(), putTx.Hash, "put.Hash does not match local hash")

	badPutTx := []byte("malformed")
	badPut, err := client.BroadcastTxAsync(ctx, badPutTx)
	require.NoError(t, err) // no API error - failure encoded in response
	require.NotEqual(t, badPut.Code, abcitypes.CodeTypeOK, "badPut.Code is OK")

	checkTicker := time.NewTicker(time.Duration(l1BlockTime) * time.Second)
	defer checkTicker.Stop()
	for range checkTicker.C {
		latestBlock, err := monomerClient.BlockByNumber(context.Background(), nil)
		require.NoError(t, err)
		if latestBlock.NumberU64() >= targetHeight {
			break
		}
	}
	t.Log("Monomer can sync")

	getTx, err := client.Tx(ctx, bftTx.Hash(), false)

	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, getTx.TxResult.Code, "txResult.Code is not OK")
	require.Equal(t, bftTx, getTx.Tx, "txBytes do not match")

	txBlock, err := monomerClient.BlockByNumber(ctx, big.NewInt(getTx.Height))
	require.NoError(t, err)
	require.Len(t, txBlock.Transactions(), 2)

	for i := uint64(2); i < targetHeight; i++ {
		block, err := monomerClient.BlockByNumber(ctx, new(big.Int).SetUint64(i))
		require.NoError(t, err)
		txs := block.Transactions()
		require.GreaterOrEqual(t, len(txs), 1, "expected at least 1 tx in block")
		if tx := txs[0]; !tx.IsDepositTx() {
			txBytes, err := tx.MarshalJSON()
			require.NoError(t, err)
			require.Fail(t, fmt.Sprintf("expected tx to be deposit tx: %s", txBytes))
		}
	}
	t.Log("Monomer blocks contain the l1 attributes deposit tx")
}

func newURL(t *testing.T, address string) *e2eurl.URL {
	stdURL, err := url.Parse(address)
	require.NoError(t, err)
	resultURL, err := e2eurl.Parse(stdURL)
	require.NoError(t, err)
	return resultURL
}
