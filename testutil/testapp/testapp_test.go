package testapp_test

import (
	"encoding/json"
	"fmt"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutil/testapp"
	"github.com/stretchr/testify/require"
)

func TestRollbackToHeight(t *testing.T) {
	chainID := monomer.ChainID(0).String()
	app := testapp.NewTest(t, chainID)
	app.InitChain(abcitypes.RequestInitChain{
		ChainId: chainID,
		AppStateBytes: func() []byte {
			got, err := json.Marshal(app.DefaultGenesis())
			require.NoError(t, err)
			return got
		}(),
		InitialHeight: 1,
	})
	app.Commit()

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

	require.NoError(t, app.RollbackToHeight(uint64(newHeight)))
	app.StateContains(t, uint64(newHeight), notReorgedState)
	app.StateDoesNotContain(t, uint64(newHeight), reorgedState)

	{
		reorgInfo := app.Info(abcitypes.RequestInfo{})
		require.Equal(t, int64(newHeight), reorgInfo.GetLastBlockHeight())
	}

	newState := make(map[string]string)
	for i := newHeight+1; i < height; i++ {
		key, value := build(t, app, chainID, int64(i))
		newState[key] = value
	}
	{
		info := app.Info(abcitypes.RequestInfo{})
		require.Equal(t, int64(height-1), info.GetLastBlockHeight())
	}
	app.StateContains(t, uint64(height-1), notReorgedState)
	app.StateContains(t, uint64(height-1), newState)
}

func build(t *testing.T, app *testapp.App, chainID string, height int64) (string, string) {
	app.BeginBlock(abcitypes.RequestBeginBlock{
		Header: *(&bfttypes.Header{
			Height:  height,
			ChainID: chainID,
		}).ToProto(),
	})
	key := fmt.Sprintf("k%d", height)
	value := fmt.Sprintf("v%d", height)
	resp := app.DeliverTx(abcitypes.RequestDeliverTx{
		Tx: testapp.ToTx(t, key, value),
	})
	require.True(t, resp.IsOK())
	app.EndBlock(abcitypes.RequestEndBlock{})
	app.Commit()
	return key, value
}
