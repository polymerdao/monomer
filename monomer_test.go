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

	hexID := (*hexutil.Big)(bigInt)
	require.Equal(t, hexID, id.HexBig())
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
		Height:  header.Height,
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
		Number:          big.NewInt(header.Height),
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

func TestBlock(t *testing.T) {
	header := newTestHeader()

	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	txs := ethtypes.Transactions{l1InfoTx, depositTx, cosmosEthTx}
	block := testutils.GenerateBlockFromEthTxs(t,
		l1InfoTx,
		[]*ethtypes.Transaction{depositTx},
		[]*ethtypes.Transaction{cosmosEthTx},
	)

	emptyTxs := bfttypes.Txs{}

	t.Run("NewBlock", func(t *testing.T) {
		newBlock := monomer.NewBlock(header, emptyTxs)

		require.Equal(t, &monomer.Block{
			Header: header,
			Txs:    emptyTxs,
		}, newBlock)

		require.Panics(t, func() { monomer.NewBlock(nil, emptyTxs) })
		require.Panics(t, func() { monomer.NewBlock(header, nil) })
		require.Panics(t, func() { monomer.NewBlock(nil, nil) })
	})

	t.Run("SetHeader", func(t *testing.T) {
		block := monomer.NewBlock(header, emptyTxs)
		ethBlock, err := block.ToEth()
		require.NoError(t, err)

		newBlock, err := monomer.SetHeader(block)
		require.NoError(t, err)
		require.Equal(t, ethBlock.Hash(), newBlock.Header.Hash)

		_, err = monomer.SetHeader(nil)
		require.Error(t, err)
	})

	t.Run("MakeBlock", func(t *testing.T) {
		block := monomer.NewBlock(header, emptyTxs)
		ethBlock, err := block.ToEth()
		require.NoError(t, err)

		newBlock, err := monomer.MakeBlock(header, emptyTxs)
		require.NoError(t, err)

		require.Equal(t, ethBlock.Hash(), newBlock.Header.Hash)
	})

	t.Run("ToEth", func(t *testing.T) {
		ethBlock, err := block.ToEth()
		require.NoError(t, err)
		for i, ethTx := range txs {
			require.EqualExportedValues(t, ethTx, ethBlock.Body().Transactions[i])
		}

		newBlock := monomer.NewBlock(header, bfttypes.Txs{[]byte("transaction1"), []byte("transaction2")})
		_, err = newBlock.ToEth()
		require.Error(t, err)

		newBlock = nil
		_, err = newBlock.ToEth()
		require.Error(t, err)
	})

	t.Run("ToCometLikeBlock", func(t *testing.T) {
		cometLikeBlock := block.ToCometLikeBlock()
		require.Equal(t, &bfttypes.Block{
			Header: bfttypes.Header{
				ChainID: block.Header.ChainID.String(),
				Time:    time.Unix(int64(block.Header.Time), 0),
				Height:  block.Header.Height,
				AppHash: block.Header.StateRoot.Bytes(),
			},
		}, cometLikeBlock)
	})
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

func TestPayloadAttributes(t *testing.T) {
	t.Run("ID NoTxPool=false", func(t *testing.T) {
		pa := newPayloadAttributes()

		id := pa.ID()
		require.NotNil(t, id)

		newID := pa.ID()
		require.Equal(t, id, newID)
	})

	t.Run("ID NoTxPool=true", func(t *testing.T) {
		pa := newPayloadAttributes()

		id := pa.ID()
		require.NotNil(t, id)
	})

	t.Run("ValidForkchoiceUpdateResult", func(t *testing.T) {
		headBlockHash := common.HexToHash("0x1")
		payloadID := &engine.PayloadID{}

		result := monomer.ValidForkchoiceUpdateResult(&headBlockHash, payloadID)
		require.Equal(t, opeth.ExecutionValid, result.PayloadStatus.Status)
		require.Equal(t, headBlockHash.Bytes(), result.PayloadStatus.LatestValidHash.Bytes())
		require.Equal(t, payloadID, result.PayloadID)
		require.Equal(t, &opeth.ForkchoiceUpdatedResult{
			PayloadStatus: opeth.PayloadStatusV1{
				Status:          opeth.ExecutionValid,
				LatestValidHash: &headBlockHash,
			},
			PayloadID: payloadID,
		}, result)
	})
}
