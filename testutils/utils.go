package testutils

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/stretchr/testify/require"
)

func NewCometMemDB(t *testing.T) cometdb.DB {
	db := cometdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func NewMemDB(t *testing.T) dbm.DB {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func NewEthStateDB(t *testing.T) state.Database {
	rawstatedb := rawdb.NewMemoryDatabase()
	ethstatedb := state.NewDatabase(rawstatedb)
	t.Cleanup(func() {
		require.NoError(t, rawstatedb.Close())
		require.NoError(t, ethstatedb.TrieDB().Close())
	})
	return ethstatedb
}

func NewLocalMemDB(t *testing.T) *localdb.DB {
	db, err := pebble.Open("", &pebble.Options{
		FS: vfs.NewMem(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return localdb.New(db)
}

// GenerateEthTxs generates an L1 attributes tx, deposit tx, and cosmos tx packed in an Ethereum transaction.
// The transactions are not meant to be executed.
func GenerateEthTxs(t *testing.T) (*gethtypes.Transaction, *gethtypes.Transaction, *gethtypes.Transaction) {
	timestamp := uint64(0)
	l1Block := gethtypes.NewBlock(&gethtypes.Header{
		BaseFee:    big.NewInt(10),
		Difficulty: common.Big0,
		Number:     big.NewInt(0),
		Time:       timestamp,
	}, nil, nil, nil, trie.NewStackTrie(nil))
	l1InfoRawTx, err := derive.L1InfoDeposit(&rollup.Config{
		Genesis:   rollup.Genesis{L2: eth.BlockID{Number: 0}},
		L2ChainID: big.NewInt(1234),
	}, eth.SystemConfig{}, 0, eth.BlockToInfo(l1Block), timestamp)
	require.NoError(t, err)
	l1InfoTx := gethtypes.NewTx(l1InfoRawTx)

	rng := rand.New(rand.NewSource(1234))
	depositTx := gethtypes.NewTx(testutils.GenerateDeposit(testutils.RandomHash(rng), rng))

	cosmosEthTx := monomer.AdaptNonDepositCosmosTxToEthTx([]byte{1})
	return l1InfoTx, depositTx, cosmosEthTx
}

func cosmosTxsFromEthTxs(t *testing.T, l1InfoTx *gethtypes.Transaction, depositTxs, cosmosEthTxs []*gethtypes.Transaction) bfttypes.Txs {
	l1InfoTxBytes, err := l1InfoTx.MarshalBinary()
	require.NoError(t, err)
	ethTxBytes := []hexutil.Bytes{l1InfoTxBytes}
	for _, depositTx := range depositTxs {
		depositTxBytes, err := depositTx.MarshalBinary()
		require.NoError(t, err)
		ethTxBytes = append(ethTxBytes, depositTxBytes)
	}
	for _, cosmosEthTx := range cosmosEthTxs {
		cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
		require.NoError(t, err)
		ethTxBytes = append(ethTxBytes, cosmosEthTxBytes)
	}
	cosmosTxs, err := monomer.AdaptPayloadTxsToCosmosTxs(ethTxBytes, nil, "")
	require.NoError(t, err)
	return cosmosTxs
}

func GenerateBlockFromEthTxs(t *testing.T, l1InfoTx *gethtypes.Transaction, depositTxs, cosmosEthTxs []*gethtypes.Transaction) *monomer.Block {
	cosmosTxs := cosmosTxsFromEthTxs(t, l1InfoTx, depositTxs, cosmosEthTxs)
	block, err := monomer.MakeBlock(&monomer.Header{}, cosmosTxs)
	require.NoError(t, err)
	return block
}

// GenerateBlock generates a valid block (up to stateless validation). The block is not meant to be executed.
func GenerateBlock(t *testing.T) *monomer.Block {
	l1InfoTx, depositTx, cosmosEthTx := GenerateEthTxs(t)
	return GenerateBlockFromEthTxs(t, l1InfoTx, []*gethtypes.Transaction{depositTx}, []*gethtypes.Transaction{cosmosEthTx})
}

// GenerateBlockWithParentAndTxs generates a child block of parent with the cosmosTxs appended to the end of its transaction list.
// The genesis block is created if parent is nil.
func GenerateBlockWithParentAndTxs(t *testing.T, parent *monomer.Header, cosmosTxs ...bfttypes.Tx) *monomer.Block {
	l1InfoTx, _, _ := GenerateEthTxs(t)
	h := &monomer.Header{}
	if parent != nil {
		h.ParentHash = parent.Hash
		h.Height = parent.Height + 1
	}
	block, err := monomer.MakeBlock(h, append(cosmosTxsFromEthTxs(t, l1InfoTx, nil, nil), cosmosTxs...))
	require.NoError(t, err)
	return block
}
