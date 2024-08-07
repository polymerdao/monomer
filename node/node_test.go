package node_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
)

const (
	engineWSAddress       = "127.0.0.1:8889"
	cometHTTPAddress      = "127.0.0.1:8890"
	prometheusHTTPAddress = "127.0.0.1:26660"
	prometheusNamespace   = "monomer"
)

func TestRun(t *testing.T) {
	chainID := monomer.ChainID(0)
	engineWS, err := net.Listen("tcp", engineWSAddress)
	require.NoError(t, err)
	cometListener, err := net.Listen("tcp", cometHTTPAddress)
	require.NoError(t, err)
	app := testapp.NewTest(t, chainID.String())
	blockdb := dbm.NewMemDB()
	defer func() {
		require.NoError(t, blockdb.Close())
	}()
	txdb := cometdb.NewMemDB()
	defer func() {
		require.NoError(t, txdb.Close())
	}()
	mempooldb := dbm.NewMemDB()
	defer func() {
		require.NoError(t, mempooldb.Close())
	}()
	ethstatedb := rawdb.NewMemoryDatabase()
	defer func() {
		require.NoError(t, ethstatedb.Close())
	}()
	n := node.New(
		app,
		&genesis.Genesis{
			ChainID:  chainID,
			AppState: testapp.MakeGenesisAppState(t, app),
		},
		engineWS,
		cometListener,
		blockdb,
		mempooldb,
		txdb,
		ethstatedb,
		&config.InstrumentationConfig{
			Prometheus:           true,
			PrometheusListenAddr: prometheusHTTPAddress,
			MaxOpenConnections:   1,
			Namespace:            prometheusNamespace,
		},
		&node.SelectiveListener{
			OnEngineHTTPServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				require.NoError(t, err)
			},
		})

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, n.Run(ctx, env))

	client, err := rpc.DialContext(ctx, "ws://"+engineWS.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	ethClient := ethclient.NewClient(client)
	chainIDBig, err := ethClient.ChainID(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(chainID), chainIDBig.Uint64())

	cometClient, err := rpc.DialContext(ctx, "http://"+cometListener.Addr().String())
	require.NoError(t, err)
	defer cometClient.Close()
	want := "hello, world"
	var msg string
	require.NoError(t, cometClient.Call(&msg, "echo", want))
	require.Equal(t, want, msg)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+prometheusHTTPAddress+"/metrics", http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	respBodyBz, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, resp.Body.Close())
	}()
	respBody := string(respBodyBz)
	require.Contains(t, respBody, "monomer_eth_method_call_count{method=\"chainId\"} 1")
}
