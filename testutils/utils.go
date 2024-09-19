package testutils

import (
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/accounts/abi"
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
	l1Block := GenerateL1Block()
	l1InfoRawTx, err := derive.L1InfoDeposit(&rollup.Config{
		Genesis:   rollup.Genesis{L2: eth.BlockID{Number: 0}},
		L2ChainID: big.NewInt(1234),
	}, eth.SystemConfig{}, 0, eth.BlockToInfo(l1Block), l1Block.Time())
	require.NoError(t, err)
	l1InfoTx := gethtypes.NewTx(l1InfoRawTx)

	rng := rand.New(rand.NewSource(1234))
	depositRawTx := testutils.GenerateDeposit(testutils.RandomHash(rng), rng)
	depositRawTx.Mint = big.NewInt(100)
	depositTx := gethtypes.NewTx(depositRawTx)

	cosmosEthTx := monomer.AdaptNonDepositCosmosTxToEthTx([]byte{1})
	return l1InfoTx, depositTx, cosmosEthTx
}

func GenerateERC20DepositTx(t *testing.T, tokenAddr, userAddr common.Address, amount *big.Int) *gethtypes.Transaction {
	crossDomainMessengerABI, err := abi.JSON(strings.NewReader(opbindings.CrossDomainMessengerMetaData.ABI))
	require.NoError(t, err)
	standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.StandardBridgeMetaData.ABI))
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(1234))

	finalizeBridgeERC20Bz, err := standardBridgeABI.Pack(
		"finalizeBridgeERC20",
		tokenAddr,                    // l1 token address
		testutils.RandomAddress(rng), // l2 token address
		testutils.RandomAddress(rng), // from
		userAddr,                     // to
		amount,                       // amount
		[]byte{},                     // extra data
	)
	require.NoError(t, err)

	relayMessageBz, err := crossDomainMessengerABI.Pack(
		"relayMessage",
		big.NewInt(0),                // nonce
		testutils.RandomAddress(rng), // sender
		testutils.RandomAddress(rng), // target
		amount,                       // value
		big.NewInt(0),                // min gas limit
		finalizeBridgeERC20Bz,        // message
	)
	require.NoError(t, err)

	to := testutils.RandomAddress(rng)
	depositTx := &gethtypes.DepositTx{
		// TODO: remove hardcoded address once a genesis state is configured
		// L2 aliased L1CrossDomainMessenger proxy address
		From: crossdomain.ApplyL1ToL2Alias(common.HexToAddress("0x9A9f2CCfdE556A7E9Ff0848998Aa4a0CFD8863AE")),
		To:   &to,
		Data: relayMessageBz,
	}
	return gethtypes.NewTx(depositTx)
}

func TxToBytes(t *testing.T, tx *gethtypes.Transaction) []byte {
	txBytes, err := tx.MarshalBinary()
	require.NoError(t, err)
	return txBytes
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
	cosmosTxs, err := monomer.AdaptPayloadTxsToCosmosTxs(ethTxBytes, generateSignTx, sdk.AccAddress("addr").String())
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

func GenerateL1Block() *gethtypes.Block {
	return gethtypes.NewBlock(&gethtypes.Header{
		BaseFee:    big.NewInt(10),
		Difficulty: common.Big0,
		Number:     big.NewInt(0),
		Time:       uint64(0),
	}, nil, nil, nil, trie.NewStackTrie(nil))
}

func generateSignTx(tx *sdktx.Tx) error {
	tx.AuthInfo = &sdktx.AuthInfo{
		Fee: &sdktx.Fee{},
	}
	return nil
}
