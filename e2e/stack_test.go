package e2e_test

import (
	"context"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/node"
	"github.com/stretchr/testify/require"
)

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	const l1BlockTime = time.Second // We would want 250ms instead, but then Anvil doesn't respond to RPCs (probably a thread starving in Anvil).

	_, verbose := os.LookupEnv("MONOMER_E2E_VERBOSE")

	contractsRootDir, err := filepath.Abs("./optimism/packages/contracts-bedrock/")
	require.NoError(t, err)

	l1URL := newURL(t, "ws://127.0.0.1:8888")
	monomerEngineURL := newURL(t, "ws://127.0.0.1:8889")
	monomerCometURL := newURL(t, "http://127.0.0.1:8890")
	opNodeURL := newURL(t, "http://127.0.0.1:8891")

	stack := e2e.New(l1URL, monomerEngineURL, monomerCometURL, opNodeURL, contractsRootDir, l1BlockTime, &e2e.SelectiveListener{
		OPLogWithPrefixCb: func(prefix string, r *log.Record) {
			if prefix != "node" && !verbose {
				return
			}
			r.Msg = prefix + ": " + r.Msg
			t.Log(string(log.TerminalFormat(false).Format(r)))
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

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()
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

	checkTicker := time.NewTicker(l1BlockTime)
	defer checkTicker.Stop()
	for range checkTicker.C {
		latestBlock, err := monomerClient.BlockByNumber(context.Background(), nil)
		require.NoError(t, err)
		if latestBlock.NumberU64() >= targetHeight {
			break
		}
	}
	t.Log("Monomer can sync")

	for i := uint64(2); i < targetHeight; i++ {
		block, err := monomerClient.BlockByNumber(ctx, new(big.Int).SetUint64(i))
		require.NoError(t, err)
		txs := block.Transactions()
		require.Len(t, txs, 1)
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
