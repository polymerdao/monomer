package node_test

import (
	"context"
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testutil/testapp"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	chainID := monomer.ChainID(0)
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	wsListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	app := testapp.NewTest(t, chainID.String())
	n := node.New(app, &genesis.Genesis{
		ChainID:  chainID,
		AppState: testapp.MakeGenesisAppState(t, app),
	}, httpListener, wsListener, rolluptypes.AdaptCosmosTxsToEthTxs, rolluptypes.AdaptPayloadTxsToCosmosTxs)

	var wg conc.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg.Go(func() {
		require.NoError(t, n.Run(ctx))
	})

	client, err := rpc.DialContext(ctx, "http://"+httpListener.Addr().String())
	require.NoError(t, err)
	ethClient := ethclient.NewClient(client)
	chainIDBig, err := ethClient.ChainID(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(chainID), chainIDBig.Uint64())
}
