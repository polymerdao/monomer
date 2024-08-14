package localdb_test

import (
	"fmt"
	"testing"

	"github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/monomerdb"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/testutils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

var labels = []eth.BlockLabel{eth.Unsafe, eth.Safe, eth.Finalized}

func TestBlockAndHeader(t *testing.T) {
	db := testutils.NewLocalMemDB(t)

	_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)

	depositTxBytes, err := depositTx.MarshalBinary()
	require.NoError(t, err)
	cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
	require.NoError(t, err)

	adaptedTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{depositTxBytes, cosmosEthTxBytes}, nil, "")
	require.NoError(t, err)

	block, err := monomer.MakeBlock(&monomer.Header{}, adaptedTxs)
	require.NoError(t, err)
	require.NoError(t, db.AppendBlock(block))

	// Labels don't exist yet.
	for _, label := range labels {
		_, err := db.BlockByLabel(label)
		require.ErrorIs(t, err, monomerdb.ErrNotFound)
		_, err = db.HeaderByLabel(label)
		require.ErrorIs(t, err, monomerdb.ErrNotFound)
	}

	// UpdateLabels
	require.NoError(t, db.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))

	testHeadBlock(t, db, block)
}

// testHeadBlock ensures block is present in the db and that it is the latest finalized, safe, and unsafe block.
func testHeadBlock(t *testing.T, db *localdb.DB, block *monomer.Block) {
	// Height
	height, err := db.Height()
	require.NoError(t, err)
	require.Equal(t, block.Header.Height, height)

	// HeadBlock
	headBlock, err := db.HeadBlock()
	require.NoError(t, err)
	require.Equal(t, block, headBlock)

	// HeadHeader
	headHeader, err := db.HeadHeader()
	require.NoError(t, err)
	require.Equal(t, block.Header, headHeader)

	// HeaderByLabel
	for _, label := range labels {
		h, err := db.HeaderByLabel(label)
		require.NoError(t, err)
		require.Equal(t, block.Header, h)
	}

	// HeaderByHeight
	_, err = db.HeaderByHeight(block.Header.Height + 1)
	require.ErrorIs(t, err, monomerdb.ErrNotFound)

	h, err := db.HeaderByHeight(block.Header.Height)
	require.NoError(t, err)
	require.Equal(t, block.Header, h)

	// HeaderByHash
	_, err = db.HeaderByHash(common.Hash{})
	require.ErrorIs(t, err, monomerdb.ErrNotFound)

	h, err = db.HeaderByHash(block.Header.Hash)
	require.NoError(t, err)
	require.Equal(t, block.Header, h)

	// BlockByLabel
	for _, label := range labels {
		b, err := db.BlockByLabel(label)
		require.NoError(t, err)
		require.Equalf(t, block, b, "label: %s", label)
	}

	// BlockByHeight
	_, err = db.BlockByHeight(block.Header.Height + 1)
	require.ErrorIs(t, err, monomerdb.ErrNotFound)

	b, err := db.BlockByHeight(block.Header.Height)
	require.NoError(t, err)
	require.Equal(t, block, b)

	// BlockByHash
	_, err = db.BlockByHash(common.Hash{})
	require.ErrorIs(t, err, monomerdb.ErrNotFound)

	b, err = db.BlockByHash(block.Header.Hash)
	require.NoError(t, err)
	require.Equal(t, block, b)
}

func TestRollback(t *testing.T) {
	db := testutils.NewLocalMemDB(t)

	_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)

	depositTxBytes, err := depositTx.MarshalBinary()
	require.NoError(t, err)
	cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
	require.NoError(t, err)

	adoptedTxsCases := make([]types.Txs, 3)

	for i := range adoptedTxsCases {
		hexTxs := make([]hexutil.Bytes, 0, 2*(i+1))
		for range i + 1 {
			hexTxs = append(hexTxs, depositTxBytes)
		}
		for range i + 1 {
			hexTxs = append(hexTxs, cosmosEthTxBytes)
		}
		adoptedTxsCases[i], err = rolluptypes.AdaptPayloadTxsToCosmosTxs(hexTxs, nil, "")
		require.NoError(t, err)
	}

	block1, err := monomer.MakeBlock(&monomer.Header{Height: 1}, adoptedTxsCases[0])
	require.NoError(t, err)
	require.NoError(t, db.AppendBlock(block1))
	require.NoError(t, db.UpdateLabels(block1.Header.Hash, block1.Header.Hash, block1.Header.Hash))

	block2, err := monomer.MakeBlock(&monomer.Header{Height: 2}, adoptedTxsCases[1])
	require.NoError(t, err)
	require.NoError(t, db.AppendBlock(block2))

	block3, err := monomer.MakeBlock(&monomer.Header{Height: 3}, adoptedTxsCases[2])
	require.NoError(t, err)
	require.NoError(t, db.AppendBlock(block3))

	require.NoError(t, db.UpdateLabels(block3.Header.Hash, block2.Header.Hash, block1.Header.Hash))
	require.NoError(t, db.Rollback(block1.Header.Hash, block1.Header.Hash, block1.Header.Hash))
	testHeadBlock(t, db, block1)

	for _, removedBlock := range []*monomer.Block{block2, block3} {
		t.Run(fmt.Sprintf("block %d rolled back", removedBlock.Header.Height), func(t *testing.T) {
			// HeaderByHeight
			_, err := db.HeaderByHeight(removedBlock.Header.Height)
			require.ErrorIs(t, err, monomerdb.ErrNotFound)

			// HeaderByHash
			_, err = db.HeaderByHash(removedBlock.Header.Hash)
			require.ErrorIs(t, err, monomerdb.ErrNotFound)

			// BlockByHeight
			_, err = db.BlockByHeight(removedBlock.Header.Height)
			require.ErrorIs(t, err, monomerdb.ErrNotFound)

			// BlockByHash
			_, err = db.BlockByHash(removedBlock.Header.Hash)
			require.ErrorIs(t, err, monomerdb.ErrNotFound)
		})
	}
}
