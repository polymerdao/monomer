package genesis_test

import (
	"encoding/json"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/testutil/testapp"
	"github.com/polymerdao/monomer/testutil/testapp/x/testmodule"
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

			blockstoredb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, blockstoredb.Close())
			})
			blockStore := store.NewBlockStore(blockstoredb)

			require.NoError(t, test.genesis.Commit(app, blockStore))

			info := app.Info(abci.RequestInfo{})

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
			block := &monomer.Block{
				Header: &monomer.Header{
					ChainID:  test.genesis.ChainID,
					Height:   info.GetLastBlockHeight(),
					Time:     test.genesis.Time,
					AppHash:  info.GetLastBlockAppHash(),
					GasLimit: 30_000_000, // We cheat a little and copy the default gas limit here.
				},
			}
			block.Hash()
			require.Equal(t, block, blockStore.BlockByNumber(info.GetLastBlockHeight()))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Unsafe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Safe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Finalized))
		})
	}
}
