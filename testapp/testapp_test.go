package testapp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/testapp"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

const (
	chainID = monomer.ChainID(0)
)

func TestRollbackToHeight(t *testing.T) {
	app := testapp.NewTest(t, chainID.String())

	blockStore := testutils.NewLocalMemDB(t)
	ethstatedb := testutils.NewEthStateDB(t)
	g := &genesis.Genesis{
		ChainID:  chainID,
		AppState: testapp.MakeGenesisAppState(t, app),
	}
	require.NoError(t, g.Commit(context.Background(), app, blockStore, ethstatedb))

	_, err := app.InitChain(context.Background(), &abcitypes.RequestInitChain{
		ChainId: chainID.String(),
		AppStateBytes: func() []byte {
			appStateBytes, err := json.Marshal(testapp.MakeGenesisAppState(t, app))
			require.NoError(t, err)
			return appStateBytes
		}(),
	})
	require.NoError(t, err)

	ctx := app.GetContext(false)
	sk, _, acc := app.TestAccount(ctx)
	// TODO: Test fails if account number is anything other than 4. Why?
	require.NoError(t, acc.SetAccountNumber(4))
	accSeq := acc.GetSequence()

	height := 10
	newHeight := height / 2
	notReorgedState := make(map[string]string)
	reorgedState := make(map[string]string)
	for i := 2; i < height; i++ {
		if key, value := build(t, app, int64(i), ctx, sk, acc, accSeq); i > newHeight {
			reorgedState[key] = value
		} else {
			notReorgedState[key] = value
			accSeq += 1
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
	// seq -= 4
	for i := newHeight + 1; i < height; i++ {
		key, value := build(t, app, int64(i), ctx, sk, acc, accSeq)
		newState[key] = value
		accSeq += 1
	}
	{
		info, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
		require.NoError(t, err)
		require.Equal(t, int64(height-1), info.GetLastBlockHeight())
	}
	app.StateContains(t, uint64(height-1), notReorgedState)
	app.StateContains(t, uint64(height-1), newState)
}

func build(t *testing.T, app *testapp.App, height int64, ctx sdk.Context, sk *secp256k1.PrivKey, acc sdk.AccountI, seq uint64) (string, string) {
	key := fmt.Sprintf("k%d", height)
	value := fmt.Sprintf("v%d", height)
	_, err := app.FinalizeBlock(context.Background(), &abcitypes.RequestFinalizeBlock{
		Txs:    [][]byte{testapp.ToTx(t, key, value, chainID.String(), sk, acc, seq, ctx)},
		Height: height,
	})
	require.NoError(t, err)
	_, err = app.Commit(context.Background(), &abcitypes.RequestCommit{})
	require.NoError(t, err)
	return key, value
}
