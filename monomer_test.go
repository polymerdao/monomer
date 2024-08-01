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
	"github.com/test-go/testify/assert"
)

func TestChainID(t *testing.T) {
	n := int64(12345)
	id := monomer.ChainID(n)

	idStr := strconv.FormatInt(n, 10)
	assert.Equal(t, idStr, id.String())

	bigInt := big.NewInt(n)
	assert.Equal(t, bigInt, id.Big())

	hexID := (*hexutil.Big)(bigInt)
	assert.Equal(t, hexID, id.HexBig())
}

func TestHeader(t *testing.T) {
	timestamp := time.Now()
	header := &monomer.Header{
		ChainID:    monomer.ChainID(12345),
		Height:     int64(67890),
		Time:       uint64(timestamp.Unix()),
		StateRoot:  common.HexToHash("0x1"),
		ParentHash: common.HexToHash("0x2"),
		GasLimit:   uint64(3000000),
		Hash:       common.HexToHash("0x3"),
	}

	t.Run("ToComet", func(t *testing.T) {
		cometHeader := header.ToComet()
		assert.Equal(t, header.ChainID.String(), cometHeader.ChainID)
		assert.Equal(t, header.Height, cometHeader.Height)
		assert.Equal(t, timestamp.Unix(), cometHeader.Time.Unix())
		assert.Equal(t, header.StateRoot.Bytes(), cometHeader.AppHash.Bytes())
	})

	t.Run("ToEth", func(t *testing.T) {
		ethHeader := header.ToEth()
		assert.Equal(t, header.Height, ethHeader.Number.Int64())
		assert.Equal(t, uint64(timestamp.Unix()), ethHeader.Time)
		assert.Equal(t, header.ParentHash, ethHeader.ParentHash)
		assert.Equal(t, header.StateRoot, ethHeader.Root)
		assert.Equal(t, header.GasLimit, ethHeader.GasLimit)
	})
}

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	f()
}

func TestBlock(t *testing.T) {
	header := &monomer.Header{
		ChainID:    12345,
		Height:     67890,
		Time:       uint64(time.Now().Unix()),
		StateRoot:  common.HexToHash("0x1"),
		ParentHash: common.HexToHash("0x2"),
		GasLimit:   3000000,
		Hash:       common.HexToHash("0x3"),
	}

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

		assert.Equal(t, header, newBlock.Header)
		assert.Equal(t, emptyTxs, newBlock.Txs)

		assertPanic(t, func() { monomer.NewBlock(nil, emptyTxs) })
		assertPanic(t, func() { monomer.NewBlock(header, nil) })
		assertPanic(t, func() { monomer.NewBlock(nil, nil) })
	})

	t.Run("SetHeader", func(t *testing.T) {
		newBlock, err := monomer.SetHeader(monomer.NewBlock(header, emptyTxs))
		assert.NoError(t, err)
		assert.NotEqual(t, common.Hash{}, newBlock.Header.Hash)

		_, err = monomer.SetHeader(nil)
		assert.Error(t, err)
	})

	t.Run("MakeBlock", func(t *testing.T) {
		block, err := monomer.MakeBlock(header, emptyTxs)
		assert.NoError(t, err)
		assert.NotEqual(t, common.Hash{}, block.Header.Hash)
	})

	t.Run("ToEth", func(t *testing.T) {
		ethBlock, err := block.ToEth()
		require.NoError(t, err)
		for i, ethTx := range txs {
			require.EqualExportedValues(t, ethTx, ethBlock.Body().Transactions[i])
		}

		newBlock := monomer.NewBlock(header, types.Txs{[]byte("transaction1"), []byte("transaction2")})
		_, err = newBlock.ToEth()
		assert.Error(t, err)

		newBlock = nil
		_, err = newBlock.ToEth()
		assert.Error(t, err)
	})

	t.Run("ToCometLikeBlock", func(t *testing.T) {
		cometLikeBlock := block.ToCometLikeBlock()
		assert.Equal(t, "0", cometLikeBlock.Header.ChainID)
		assert.Equal(t, int64(0), cometLikeBlock.Header.Height)
		assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", cometLikeBlock.Header.AppHash.String())
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
		assert.NotNil(t, id)

		newID := pa.ID()
		assert.Equal(t, id, newID)
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
		assert.NotNil(t, id)
	})

	t.Run("ValidForkchoiceUpdateResult", func(t *testing.T) {
		headBlockHash := common.HexToHash("0x1")
		payloadID := &engine.PayloadID{}

		result := monomer.ValidForkchoiceUpdateResult(&headBlockHash, payloadID)
		assert.Equal(t, opeth.ExecutionValid, result.PayloadStatus.Status)
		assert.Equal(t, headBlockHash.Bytes(), result.PayloadStatus.LatestValidHash.Bytes())
		assert.Equal(t, payloadID, result.PayloadID)
	})
}
