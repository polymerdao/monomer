package monomer_test

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/cometbft/cometbft/types"
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

	idStr := strconv.FormatInt(n, 10)
	require.Equal(t, idStr, id.String())

	bigInt := big.NewInt(n)
	require.Equal(t, bigInt, id.Big())

	hexID := (*hexutil.Big)(bigInt)
	require.Equal(t, hexID, id.HexBig())
}

func newTestHeader(timestamp uint64) *monomer.Header {
	return &monomer.Header{
		ChainID:    12345,
		Height:     67890,
		Time:       timestamp,
		StateRoot:  common.HexToHash("0x1"),
		ParentHash: common.HexToHash("0x2"),
		GasLimit:   3000000,
		Hash:       common.HexToHash("0x3"),
	}
}

func TestToComet(t *testing.T) {
	timestamp := time.Now()
	header := newTestHeader(uint64(timestamp.Unix()))
	cometHeader := header.ToComet()

	require.Equal(t, header.ChainID.String(), cometHeader.ChainID)
	require.Equal(t, header.Height, cometHeader.Height)
	require.Equal(t, timestamp.Unix(), cometHeader.Time.Unix())
	require.Equal(t, header.StateRoot.Bytes(), cometHeader.AppHash.Bytes())
}

func TestToEth(t *testing.T) {
	timestamp := time.Now()
	header := newTestHeader(uint64(timestamp.Unix()))
	ethHeader := header.ToEth()

	require.Equal(t, header.Height, ethHeader.Number.Int64())
	require.Equal(t, uint64(timestamp.Unix()), ethHeader.Time)
	require.Equal(t, header.ParentHash, ethHeader.ParentHash)
	require.Equal(t, header.StateRoot, ethHeader.Root)
	require.Equal(t, header.GasLimit, ethHeader.GasLimit)
}

func TestBlock(t *testing.T) {
	timestamp := time.Now()
	header := newTestHeader(uint64(timestamp.Unix()))

	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	txs := ethtypes.Transactions{l1InfoTx, depositTx, cosmosEthTx}
	block := testutils.GenerateBlockFromEthTxs(t,
		l1InfoTx,
		[]*ethtypes.Transaction{depositTx},
		[]*ethtypes.Transaction{cosmosEthTx},
	)

	emptyTxs := types.Txs{}

	t.Run("NewBlock", func(t *testing.T) {
		newBlock := monomer.NewBlock(header, emptyTxs)

		require.Equal(t, header, newBlock.Header)
		require.Equal(t, emptyTxs, newBlock.Txs)

		require.Panics(t, func() { monomer.NewBlock(nil, emptyTxs) })
		require.Panics(t, func() { monomer.NewBlock(header, nil) })
		require.Panics(t, func() { monomer.NewBlock(nil, nil) })
	})

	t.Run("SetHeader", func(t *testing.T) {
		newBlock, err := monomer.SetHeader(monomer.NewBlock(header, emptyTxs))
		require.NoError(t, err)
		require.NotEqual(t, common.Hash{}, newBlock.Header.Hash)

		_, err = monomer.SetHeader(nil)
		require.Error(t, err)
	})

	t.Run("MakeBlock", func(t *testing.T) {
		block, err := monomer.MakeBlock(header, emptyTxs)
		require.NoError(t, err)
		require.NotEqual(t, common.Hash{}, block.Header.Hash)
	})

	t.Run("ToEth", func(t *testing.T) {
		ethBlock, err := block.ToEth()
		require.NoError(t, err)
		for i, ethTx := range txs {
			require.EqualExportedValues(t, ethTx, ethBlock.Body().Transactions[i])
		}

		newBlock := monomer.NewBlock(header, types.Txs{[]byte("transaction1"), []byte("transaction2")})
		_, err = newBlock.ToEth()
		require.Error(t, err)

		newBlock = nil
		_, err = newBlock.ToEth()
		require.Error(t, err)
	})

	t.Run("ToCometLikeBlock", func(t *testing.T) {
		cometLikeBlock := block.ToCometLikeBlock()
		require.Equal(t, "0", cometLikeBlock.Header.ChainID)
		require.Equal(t, int64(0), cometLikeBlock.Header.Height)
		require.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", cometLikeBlock.Header.AppHash.String())
	})
}

func TestPayloadAttributes(t *testing.T) {
	t.Run("ID NoTxPool=false", func(t *testing.T) {
		pa := &monomer.PayloadAttributes{
			Timestamp:             uint64(time.Now().Unix()),
			PrevRandao:            [32]byte{1, 2, 3, 4},
			SuggestedFeeRecipient: common.HexToAddress("0x5"),
			NoTxPool:              false,
			GasLimit:              3000000,
			ParentHash:            common.HexToHash("0x2"),
			CosmosTxs:             types.Txs{[]byte("transaction1"), []byte("transaction2")},
		}

		id := pa.ID()
		require.NotNil(t, id)

		newID := pa.ID()
		require.Equal(t, id, newID)
	})

	t.Run("ID NoTxPool=true", func(t *testing.T) {
		pa := &monomer.PayloadAttributes{
			Timestamp:             uint64(time.Now().Unix()),
			PrevRandao:            [32]byte{1, 2, 3, 4},
			SuggestedFeeRecipient: common.HexToAddress("0x5"),
			NoTxPool:              true,
			GasLimit:              3000000,
			ParentHash:            common.HexToHash("0x2"),
			CosmosTxs:             types.Txs{[]byte("transaction1"), []byte("transaction2")},
		}

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
	})
}
