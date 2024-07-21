package store

/*
import (
	"fmt"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/node/node.go"
	"github.com/stretchr/testify/require"
)

func TestSetUnsafeBlock(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB())
	require.NotNil(t, bs)
	block := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     1,
			ParentHash: common.HexToHash("0x0"),
			Hash:       common.HexToHash("0x1"),
		},
	}

	bs.AddBlock(block)

	// get by hash
	byhash := bs.BlockByHash(block.Hash())
	require.NotNil(t, byhash)
	require.Equal(t, block, byhash)

	// get by number
	bynumber := bs.BlockByNumber(block.Header.Height)
	require.NotNil(t, bynumber)
	require.Equal(t, block, byhash)

	// get by label
	bs.UpdateLabel(eth.Unsafe, block.Hash())
	bylabel := bs.BlockByLabel(eth.Unsafe)
	require.NotNil(t, bylabel)
	require.Equal(t, block, bylabel)
}

func TestUpdateLabel(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB())
	require.NotNil(t, bs)
	block := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     1,
			ParentHash: common.HexToHash("0x0"),
			Hash:       common.HexToHash("0x1"),
		},
	}

	bs.AddBlock(block)
	bs.UpdateLabel(eth.Unsafe, block.Hash())

	require.NoError(t, bs.UpdateLabel(eth.Safe, block.Hash()))

	safe := bs.BlockByLabel(eth.Safe)
	require.NotNil(t, safe)
	require.Equal(t, block, safe)

	// unsafe label still points to the same block
	unsafe := bs.BlockByLabel(eth.Unsafe)
	require.NotNil(t, unsafe)
	require.Equal(t, block, unsafe)

	// only when a new unsafe block is set, the label is auto updated
	block2 := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     2,
			ParentHash: common.HexToHash("0x1"),
			Hash:       common.HexToHash("0x2"),
		},
	}
	bs.AddBlock(block2)
	bs.UpdateLabel(eth.Unsafe, block2.Hash())
	require.Equal(t, block2, bs.BlockByLabel(eth.Unsafe))

	// but the safe label was not updated
	require.Equal(t, block, bs.BlockByLabel(eth.Safe))
}

func TestMultipleBlocks(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB())
	require.NotNil(t, bs)
	var blocks []*eetypes.Block
	for i := int64(1); i <= 10; i++ {
		blocks = append(blocks, &eetypes.Block{
			Header: &eetypes.Header{
				Height:     i,
				ParentHash: common.HexToHash(fmt.Sprintf("0x%d", i+1)),
				Hash:       common.HexToHash(fmt.Sprintf("0x%d", i)),
			},
		})
	}

	for _, block := range blocks {
		bs.AddBlock(block)
	}

	for _, block := range blocks {
		require.Equal(t, block, bs.BlockByHash(block.Hash()))
	}

	for _, block := range blocks {
		require.Equal(t, block, bs.BlockByNumber(block.Header.Height))
	}
}

func TestBlockNotFoundError(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB())
	require.NotNil(t, bs)

	require.Nil(t, bs.BlockByLabel(eth.Unsafe))
	require.Nil(t, bs.BlockByLabel(eth.Safe))
	require.Nil(t, bs.BlockByLabel(eth.Finalized))

	require.Nil(t, bs.BlockByHash([32]byte{}))
	require.Nil(t, bs.BlockByNumber(42))

	require.Error(t, bs.UpdateLabel(eth.Safe, [32]byte{}))
}

func TestRollback(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB())
	require.NotNil(t, bs)
	var blocks []*eetypes.Block
	for i := int64(1); i <= 10; i++ {
		blocks = append(blocks, &eetypes.Block{
			Header: &eetypes.Header{
				Height:     i,
				ParentHash: common.HexToHash(fmt.Sprintf("0x%d", i+1)),
				Hash:       common.HexToHash(fmt.Sprintf("0x%d", i)),
			},
		})
	}

	for _, block := range blocks {
		bs.AddBlock(block)
	}

	require.NoError(t, bs.RollbackToHeight(5))

	for _, block := range blocks {
		if block.Header.Height > 5 {
			require.Nil(t, bs.BlockByNumber(block.Header.Height))
			require.Nil(t, bs.BlockByHash(block.Hash()))
		} else {
			require.NotNil(t, bs.BlockByNumber(block.Header.Height))
			require.NotNil(t, bs.BlockByHash(block.Hash()))
		}
	}

	// trying to rollback to a height that does not exist is noop
	require.NoError(t, bs.RollbackToHeight(6))
}
*/
