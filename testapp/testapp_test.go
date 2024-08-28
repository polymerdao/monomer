package testapp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
)

func TestRollbackToHeight(t *testing.T) {
	chainID := monomer.ChainID(0).String()
	app := testapp.NewTest(t, chainID)
	_, err := app.InitChain(context.Background(), &abcitypes.RequestInitChain{
		ChainId: chainID,
		AppStateBytes: func() []byte {
			got, err := json.Marshal(app.DefaultGenesis())
			require.NoError(t, err)
			return got
		}(),
		InitialHeight: 1,
	})
	require.NoError(t, err)
	_, err = app.Commit(context.Background(), &abcitypes.RequestCommit{})
	require.NoError(t, err)

	height := 10
	newHeight := height / 2
	notReorgedState := make(map[string]string)
	reorgedState := make(map[string]string)
	for i := 2; i < height; i++ {
		if key, value := build(t, app, chainID, int64(i)); i > newHeight {
			reorgedState[key] = value
		} else {
			notReorgedState[key] = value
		}
	}

	require.NoError(t, app.RollbackToHeight(context.Background(), uint64(newHeight)))
	app.StateContains(t, uint64(newHeight), notReorgedState)
	app.StateDoesNotContain(t, uint64(newHeight), reorgedState)

	{
		reorgInfo, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
		require.NoError(t, err)
		require.Equal(t, int64(newHeight), reorgInfo.GetLastBlockHeight())
	}

	newState := make(map[string]string)
	for i := newHeight + 1; i < height; i++ {
		key, value := build(t, app, chainID, int64(i))
		newState[key] = value
	}
	{
		info, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
		require.NoError(t, err)
		require.Equal(t, int64(height-1), info.GetLastBlockHeight())
	}
	app.StateContains(t, uint64(height-1), notReorgedState)
	app.StateContains(t, uint64(height-1), newState)
}

func build(t *testing.T, app *testapp.App, chainID string, height int64) (string, string) {
	key := fmt.Sprintf("k%d", height)
	value := fmt.Sprintf("v%d", height)
	_, err := app.FinalizeBlock(context.Background(), &abcitypes.RequestFinalizeBlock{
		Txs:    [][]byte{testapp.ToTestTx(t, key, value)},
		Height: height,
	})
	require.NoError(t, err)
	_, err = app.Commit(context.Background(), &abcitypes.RequestCommit{})
	require.NoError(t, err)
	return key, value
}
