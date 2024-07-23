package testutils

import (
	"math/big"
	"math/rand"
	"testing"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
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

// GenerateEthTxs generates an L1 attributes tx, deposit tx, and cosmos tx packed in an Ethereum transaction.
// The transactions are not meant to be executed.
func GenerateEthTxs(t *testing.T) (*types.Transaction, *types.Transaction, *types.Transaction) {
	timestamp := uint64(0)
	l1Block := types.NewBlock(&types.Header{
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
	l1InfoTx := types.NewTx(l1InfoRawTx)

	rng := rand.New(rand.NewSource(1234))
	depositTx := types.NewTx(testutils.GenerateDeposit(testutils.RandomHash(rng), rng))

	cosmosEthTx := rolluptypes.AdaptNonDepositCosmosTxToEthTx([]byte{1})
	return l1InfoTx, depositTx, cosmosEthTx
}

func GenerateBlockFromEthTxs(t *testing.T, l1InfoTx *types.Transaction, depositTxs, cosmosEthTxs []*types.Transaction) *monomer.Block {
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
	cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs(ethTxBytes)
	require.NoError(t, err)
	return &monomer.Block{
		Header: &monomer.Header{},
		Txs:    cosmosTxs,
	}
}

// GenerateBlock generates a valid block (up to stateless validation). The block is not meant to be executed.
func GenerateBlock(t *testing.T) *monomer.Block {
	l1InfoTx, depositTx, cosmosEthTx := GenerateEthTxs(t)
	return GenerateBlockFromEthTxs(t, l1InfoTx, []*types.Transaction{depositTx}, []*types.Transaction{cosmosEthTx})
}
