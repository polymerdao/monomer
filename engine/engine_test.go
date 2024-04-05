package engine_test

import (
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	ethengine "github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutil/testapp"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

// newAPI returns the block store and builder as well so they can be manipulated and introspected in tests.
func newAPI(t *testing.T) (*engine.EngineAPI, *builder.Builder, store.BlockStore) {
	mempooldb := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, mempooldb.Close())
	})
	pool := mempool.New(mempooldb)

	blockdb := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, blockdb.Close())
	})
	blockStore := store.NewBlockStore(blockdb)

	txdb := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, txdb.Close())
	})
	txStore := txstore.NewTxStore(txdb)

	g := &genesis.Genesis{}

	app := testapp.NewTest(t, g.ChainID.String())

	eventBus := bfttypes.NewEventBus()
	require.NoError(t, eventBus.Start())
	t.Cleanup(func() {
		require.NoError(t, eventBus.Stop())
	})

	require.NoError(t, g.Commit(app, blockStore))

	b := builder.New(
		pool,
		app,
		blockStore,
		txStore,
		eventBus,
		g.ChainID,
	)
	return engine.NewEngineAPI(b, app, rolluptypes.AdaptPayloadTxsToCosmosTxs, blockStore), b, blockStore
}

func TestNewPayloadV3(t *testing.T) {
	api, _, blockStore := newAPI(t)

	_, err := api.NewPayloadV3(eth.ExecutionPayload{})
	require.ErrorContains(t, err, ethengine.InvalidParams.Error())

	headBlockHash := blockStore.HeadBlock().Hash()
	payloadStatus, err := api.NewPayloadV3(eth.ExecutionPayload{
		BlockHash: headBlockHash,
	})
	require.NoError(t, err)
	require.Equal(t, &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, payloadStatus)
}

func TestForkchoiceUpdatedV3(t *testing.T) {
	// TODO maybe read spec first.
	// cases:
	//   - before: block after genesis has no labels
	//     after: block after genesis has labels (all permutations), [with and without payload attributes]
	//       after: labels rolled back to genesis, [with and without payload attributes]

	api, b, blockStore := newAPI(t)
	genesisBlock := blockStore.HeadBlock()
	require.NoError(t, b.Build(&builder.Payload{
		Timestamp: genesisBlock.Header.Time + 1,
	}))
	postGenesisBlock := blockStore.HeadBlock()

	for description, fcu := range map[string]eth.ForkchoiceState{
		"unsafe, safe, finalized": {
			HeadBlockHash:      postGenesisBlock.Hash(),
			SafeBlockHash:      postGenesisBlock.Hash(),
			FinalizedBlockHash: postGenesisBlock.Hash(),
		},
		"unsafe, safe": {
			HeadBlockHash: postGenesisBlock.Hash(),
			SafeBlockHash: postGenesisBlock.Hash(),
		},
		"unsafe": {
			HeadBlockHash: postGenesisBlock.Hash(),
		},
	} {
		t.Run(description, func(t *testing.T) {
			// just manually do these
			for description, pa := range map[string]eth.PayloadAttributes{
				"with payload": {
					// TODO add test
				},
				"without payload": {},
			} {
				t.Run(description, func(t *testing.T) {
					result, err := api.ForkchoiceUpdatedV3(fcu, &pa)
					require.NoError(t, err)
					postGenesisBlockHash := postGenesisBlock.Hash()
					require.Equal(t, monomer.ValidForkchoiceUpdateResult(&postGenesisBlockHash, nil), result)
				})
			}
		})
	}

	result, err := api.ForkchoiceUpdatedV3(eth.ForkchoiceState{
		HeadBlockHash: postGenesisBlock.Hash(),
	}, &eth.PayloadAttributes{})
	require.NoError(t, err)
	postGenesisBlockHash := postGenesisBlock.Hash()
	require.Equal(t, monomer.ValidForkchoiceUpdateResult(&postGenesisBlockHash, nil), result)

	result, err = api.ForkchoiceUpdatedV3(eth.ForkchoiceState{
		HeadBlockHash: genesisBlock.Hash(),
	}, &eth.PayloadAttributes{})
	require.NoError(t, err)
	genesisBlockHash := genesisBlock.Hash()
	require.Equal(t, monomer.ValidForkchoiceUpdateResult(&genesisBlockHash, nil), result)
}

func TestGetPayloadV3(t *testing.T) {
	// cases:
	//   - payload was not introduced from ForkchoiceUpdated call.
	//   - payload was introduced, but is not current
	//   - payload is current and is built and committed

	// TODO: check what the engine api says to validate about payload. e.g. timestamp
}
