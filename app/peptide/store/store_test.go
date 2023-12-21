package store

import (
	"fmt"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/stretchr/testify/require"
)

type mockBlock struct {
	height int64
	hash   string
	parent string
}

var _ eetypes.BlockData = (*mockBlock)(nil)

func (m *mockBlock) Height() int64 {
	return m.height
}

func (m *mockBlock) Bytes() []byte {
	return []byte(fmt.Sprintf("%v %v %v", m.hash, m.parent, m.height))
}

func (m *mockBlock) Hash() eetypes.Hash {
	var h eetypes.Hash
	copy(h[:], m.hash)
	return h
}

func (m *mockBlock) ParentHash() eetypes.Hash {
	var h eetypes.Hash
	copy(h[:], m.parent)
	return h
}

func Unmarshal(bytes []byte) (eetypes.BlockData, error) {
	m := mockBlock{}
	matched, err := fmt.Sscanf(string(bytes), "%v %v %v", &m.hash, &m.parent, &m.height)
	if err != nil || matched != 3 {
		panic("could not parse bytes:" + string(bytes))
	}
	return &m, nil
}

func TestSetUnsafeBlock(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB(), Unmarshal)
	require.NotNil(t, bs)
	block := mockBlock{height: 1, hash: "hash", parent: "parent"}

	bs.AddBlock(&block)

	// get by hash
	byhash := bs.BlockByHash(block.Hash())
	require.NotNil(t, byhash)
	require.Equal(t, block.Bytes(), byhash.Bytes())

	// get by number
	bynumber := bs.BlockByNumber(block.Height())
	require.NotNil(t, bynumber)
	require.Equal(t, block.Bytes(), byhash.Bytes())

	// get by label
	bs.UpdateLabel(eth.Unsafe, block.Hash())
	bylabel := bs.BlockByLabel(eth.Unsafe)
	require.NotNil(t, bylabel)
	require.Equal(t, block.Bytes(), bylabel.Bytes())
}

func TestUpdateLabel(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB(), Unmarshal)
	require.NotNil(t, bs)
	block := mockBlock{height: 1, hash: "hash", parent: "parent"}

	bs.AddBlock(&block)
	bs.UpdateLabel(eth.Unsafe, block.Hash())

	require.NoError(t, bs.UpdateLabel(eth.Safe, block.Hash()))

	safe := bs.BlockByLabel(eth.Safe)
	require.NotNil(t, safe)
	require.Equal(t, block.Bytes(), safe.Bytes())

	// unsafe label still points to the same block
	unsafe := bs.BlockByLabel(eth.Unsafe)
	require.NotNil(t, unsafe)
	require.Equal(t, block.Bytes(), unsafe.Bytes())

	// only when a new unsafe block is set, the label is auto updated
	block2 := mockBlock{height: 2, hash: "hash2", parent: "parent2"}
	bs.AddBlock(&block2)
	bs.UpdateLabel(eth.Unsafe, block2.Hash())
	require.Equal(t, block2.Bytes(), bs.BlockByLabel(eth.Unsafe).Bytes())

	// but the safe label was not updated
	require.Equal(t, block.Bytes(), bs.BlockByLabel(eth.Safe).Bytes())
}

func TestMultipleBlocks(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB(), Unmarshal)
	require.NotNil(t, bs)
	var blocks []mockBlock
	for i := int64(1); i <= 10; i++ {
		blocks = append(blocks, mockBlock{height: i, hash: fmt.Sprintf("h:%d", i), parent: fmt.Sprintf("p:%d", i)})
	}

	for _, block := range blocks {
		bs.AddBlock(&block)
	}

	for _, block := range blocks {
		require.Equal(t, block.Bytes(), bs.BlockByHash(block.Hash()).Bytes())
	}

	for _, block := range blocks {
		require.Equal(t, block.Bytes(), bs.BlockByNumber(block.Height()).Bytes())
	}
}

func TestBlockNotFoundError(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB(), Unmarshal)
	require.NotNil(t, bs)

	require.Nil(t, bs.BlockByLabel(eth.Unsafe))
	require.Nil(t, bs.BlockByLabel(eth.Safe))
	require.Nil(t, bs.BlockByLabel(eth.Finalized))

	require.Nil(t, bs.BlockByHash([32]byte{}))
	require.Nil(t, bs.BlockByNumber(42))

	require.Error(t, bs.UpdateLabel(eth.Safe, [32]byte{}))
}

func TestRollback(t *testing.T) {
	bs := NewBlockStore(dbm.NewMemDB(), Unmarshal)
	require.NotNil(t, bs)
	var blocks []mockBlock
	for i := int64(1); i <= 10; i++ {
		blocks = append(blocks, mockBlock{height: i, hash: fmt.Sprintf("h:%d", i), parent: fmt.Sprintf("p:%d", i)})
	}

	for _, block := range blocks {
		bs.AddBlock(&block)
	}

	require.NoError(t, bs.RollbackToHeight(5))

	for _, block := range blocks {
		if block.Height() > 5 {
			require.Nil(t, bs.BlockByNumber(block.Height()))
			require.Nil(t, bs.BlockByHash(block.Hash()))
		} else {
			require.NotNil(t, bs.BlockByNumber(block.Height()))
			require.NotNil(t, bs.BlockByHash(block.Hash()))
		}
	}

	// trying to rollback to a height that does not exist is noop
	require.NoError(t, bs.RollbackToHeight(6))
}
