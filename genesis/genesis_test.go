package genesis_test

import (
	"context"
	"encoding/json"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/testapp"
	"github.com/polymerdao/monomer/testapp/x/testmodule"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestCommit(t *testing.T) {
	tests := map[string]struct {
		kvs     []string
		genesis *genesis.Genesis
	}{
		"nonempty testmodule state": {
			kvs:     []string{"k1", "v1"},
			genesis: &genesis.Genesis{},
		},
		"non-zero chain ID": {
			genesis: &genesis.Genesis{
				ChainID: 1,
			},
		},
		"non-zero genesis time": {
			genesis: &genesis.Genesis{
				Time: 1,
			},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			app := testapp.NewTest(t, test.genesis.ChainID.String())
			test.genesis.AppState = testapp.MakeGenesisAppState(t, app, test.kvs...)

			blockstoredb := dbm.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, blockstoredb.Close())
			})
			blockStore := store.NewBlockStore(blockstoredb)
			ethstatedb := testutils.NewEthStateDB(t)

			require.NoError(t, test.genesis.Commit(context.Background(), app, blockStore, ethstatedb))

			info, err := app.Info(context.Background(), &abci.RequestInfo{})
			require.NoError(t, err)

			// Application.
			require.Equal(t, int64(1), info.GetLastBlockHeight()) // This means that the genesis height was set correctly.
			{
				// Ensure testmodule state was set correctly.
				kvs := make(map[string]string)
				require.NoError(t, json.Unmarshal(test.genesis.AppState[testmodule.ModuleName], &kvs))
				app.StateContains(t, uint64(info.GetLastBlockHeight()), kvs)
			}
			// Even though RequestInitChain contains the chain ID, we can't test that it was set properly since the ABCI doesn't expose it.

			// Block store.
			block, err := monomer.MakeBlock(&monomer.Header{
				ChainID:   test.genesis.ChainID,
				Height:    info.GetLastBlockHeight(),
				Time:      test.genesis.Time,
				GasLimit:  30_000_000, // We cheat a little and copy the default gas limit here.
				StateRoot: gethtypes.EmptyRootHash,
			}, bfttypes.Txs{})
			require.NoError(t, err)
			require.Equal(t, block, blockStore.BlockByNumber(info.GetLastBlockHeight()))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Unsafe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Safe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Finalized))
		})
	}
}
