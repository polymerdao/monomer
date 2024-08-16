package monomer_test

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	bfttypes "github.com/cometbft/cometbft/types"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestChainID(t *testing.T) {
	n := int64(12345)
	id := monomer.ChainID(n)

	require.Equal(t, strconv.FormatInt(n, 10), id.String())

	bigInt := big.NewInt(n)
	require.Equal(t, bigInt, id.Big())

	require.Equal(t, (*hexutil.Big)(bigInt), id.HexBig())
}

func newTestHeader() *monomer.Header {
	return &monomer.Header{
		ChainID:    12345,
		Height:     67890,
		Time:       uint64(time.Now().Unix()),
		StateRoot:  common.HexToHash("0x1"),
		ParentHash: common.HexToHash("0x2"),
		GasLimit:   3000000,
		Hash:       common.HexToHash("0x3"),
	}
}

func TestToComet(t *testing.T) {
	header := newTestHeader()
	cometHeader := header.ToComet()

	require.Equal(t, &bfttypes.Header{
		ChainID: header.ChainID.String(),
		Height:  int64(header.Height),
		Time:    time.Unix(int64(header.Time), 0),
		AppHash: header.StateRoot.Bytes(),
	}, cometHeader)
}

func TestToEth(t *testing.T) {
	header := newTestHeader()
	ethHeader := header.ToEth()

	require.Equal(t, &ethtypes.Header{
		ParentHash:      header.ParentHash,
		Root:            header.StateRoot,
		Number:          big.NewInt(int64(header.Height)),
		GasLimit:        header.GasLimit,
		MixDigest:       common.Hash{},
		Time:            header.Time,
		UncleHash:       ethtypes.EmptyUncleHash,
		ReceiptHash:     ethtypes.EmptyReceiptsHash,
		BaseFee:         common.Big0,
		WithdrawalsHash: &ethtypes.EmptyWithdrawalsHash,
		Difficulty:      common.Big0,
	}, ethHeader)
}

func TestBlockNewBlock(t *testing.T) {
	block := monomer.NewBlock(newTestHeader(), bfttypes.Txs{})
	ethBlock, err := block.ToEth()
	require.NoError(t, err)

	newBlock, err := monomer.SetHeader(block)
	require.NoError(t, err)
	require.Equal(t, ethBlock.Hash(), newBlock.Header.Hash)
}

func TestBlockSetHeader(t *testing.T) {
	block := monomer.NewBlock(newTestHeader(), bfttypes.Txs{})
	ethBlock, err := block.ToEth()
	require.NoError(t, err)

	newBlock, err := monomer.SetHeader(block)
	require.NoError(t, err)
	require.Equal(t, ethBlock.Hash(), newBlock.Header.Hash)
}

func TestBlockMakeBlock(t *testing.T) {
	header := newTestHeader()
	emptyTxs := bfttypes.Txs{}

	block := monomer.NewBlock(header, emptyTxs)
	ethBlock, err := block.ToEth()
	require.NoError(t, err)

	newBlock, err := monomer.MakeBlock(header, emptyTxs)
	require.NoError(t, err)

	require.Equal(t, ethBlock.Hash(), newBlock.Header.Hash)
}

func TestBlockToEth(t *testing.T) {
	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	block := testutils.GenerateBlockFromEthTxs(t,
		l1InfoTx,
		[]*ethtypes.Transaction{depositTx},
		[]*ethtypes.Transaction{cosmosEthTx},
	)

	ethTxs, err := monomer.AdaptCosmosTxsToEthTxs(block.Txs)
	require.NoError(t, err)

	actualEthBlock, err := block.ToEth()
	require.NoError(t, err)

	require.Equal(t, ethtypes.NewBlockWithWithdrawals(
		block.Header.ToEth(),
		ethTxs,
		nil,
		[]*ethtypes.Receipt{},
		[]*ethtypes.Withdrawal{},
		trie.NewStackTrie(nil),
	).Hash(), actualEthBlock.Hash())
}

// TestBlockToEthFailsWithWrongTxs tests the behavior of the Block.ToEth() method when provided with incorrect transactions.
// It creates a new block with a test header and a list of transactions containing two byte slices.
// The ToEth() method is then called on the new block, and the returned value and error are captured.
// The test asserts that an error is returned, indicating that the conversion to Ethereum format failed.
// This test is used to ensure the correct handling of invalid transactions in the Block.ToEth() method.
func TestBlockToEthFailsWithWrongTxs(t *testing.T) {
	newBlock := monomer.NewBlock(newTestHeader(), bfttypes.Txs{[]byte("transaction1"), []byte("transaction2")})
	_, err := newBlock.ToEth()
	require.Error(t, err)
}

func TestBlockToCometLikeBlock(t *testing.T) {
	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	block := testutils.GenerateBlockFromEthTxs(t,
		l1InfoTx,
		[]*ethtypes.Transaction{depositTx},
		[]*ethtypes.Transaction{cosmosEthTx},
	)

	require.Equal(t, &bfttypes.Block{
		Header: bfttypes.Header{
			ChainID: block.Header.ChainID.String(),
			Time:    time.Unix(int64(block.Header.Time), 0),
			Height:  int64(block.Header.Height),
			AppHash: block.Header.StateRoot.Bytes(),
		},
		Data: bfttypes.Data{
			Txs: block.Txs,
		},
	}, block.ToCometLikeBlock())
}

func newPayloadAttributes() *monomer.PayloadAttributes {
	return &monomer.PayloadAttributes{
		Timestamp:             uint64(time.Now().Unix()),
		PrevRandao:            [32]byte{1, 2, 3, 4},
		SuggestedFeeRecipient: common.HexToAddress("0x5"),
		NoTxPool:              false,
		GasLimit:              3000000,
		ParentHash:            common.HexToHash("0x2"),
		CosmosTxs:             bfttypes.Txs{[]byte("transaction1"), []byte("transaction2")},
	}
}

func TestPayloadAttributesID(t *testing.T) {
	tests := []struct {
		name     string
		noTxPool bool
		expected *engine.PayloadID
	}{
		{
			name: "NoTxPool is false",
		},
		{
			name:     "NoTxPool is true",
			noTxPool: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pa := newPayloadAttributes()
			pa.NoTxPool = test.noTxPool

			id := pa.ID()
			require.NotNil(t, id)
			require.Equal(t, id, pa.ID())
		})
	}
}

func TestPayloadAttributesValidForkchoiceUpdateResult(t *testing.T) {
	headBlockHash := common.HexToHash("0x1")
	payloadID := &engine.PayloadID{}

	result := monomer.ValidForkchoiceUpdateResult(&headBlockHash, payloadID)
	require.Equal(t, &opeth.ForkchoiceUpdatedResult{
		PayloadStatus: opeth.PayloadStatusV1{
			Status:          opeth.ExecutionValid,
			LatestValidHash: &headBlockHash,
		},
		PayloadID: payloadID,
	}, result)
}
