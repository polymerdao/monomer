package monomer

import (
	"errors"
	"fmt"
	"time"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

var errL1AttributesNotFound = errors.New("L1 attributes tx not found")

// AdaptPayloadTxsToCosmosTxs assumes the deposit transactions come first.
func AdaptPayloadTxsToCosmosTxs(ethTxs []hexutil.Bytes) (bfttypes.Txs, error) {
	if len(ethTxs) == 0 {
		return bfttypes.Txs{}, nil
	}

	numDepositTxs, err := countDepositTransactions(ethTxs)
	if err != nil {
		return nil, fmt.Errorf("count deposit transactions: %v", err)
	}

	depositTx, err := packDepositTxsToCosmosTx(ethTxs[:numDepositTxs], "")
	if err != nil {
		return nil, fmt.Errorf("pack deposit txs: %v", err)
	}
	depositTxBytes, err := depositTx.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal tx: %v", err)
	}

	cosmosNonDepositTxs, err := convertToCosmosNonDepositTxs(ethTxs[numDepositTxs:])
	if err != nil {
		return nil, fmt.Errorf("convert to cosmos txs: %v", err)
	}

	return append(bfttypes.Txs{depositTxBytes}, cosmosNonDepositTxs...), nil
}

func countDepositTransactions(ethTxs []hexutil.Bytes) (int, error) {
	var numDepositTxs int
	for _, txBytes := range ethTxs {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			return 0, fmt.Errorf("unmarshal binary: %v", err)
		}
		if tx.IsDepositTx() {
			numDepositTxs++
		} else {
			break // Assume deposit transactions must come first.
		}
	}
	if numDepositTxs == 0 {
		return 0, errL1AttributesNotFound
	}

	return numDepositTxs, nil
}

func packDepositTxsToCosmosTx(ethDepositTxs []hexutil.Bytes, _ string) (*rolluptypes.DepositsTx, error) {
	var ethL1AttributesTx ethtypes.Transaction
	if err := ethL1AttributesTx.UnmarshalBinary(ethDepositTxs[0]); err != nil {
		return nil, fmt.Errorf("unmarshal binary: %v", err)
	}
	l1BlockInfo, err := derive.L1BlockInfoFromBytes(&rollup.Config{}, uint64(time.Now().Unix()), ethL1AttributesTx.Data())
	if err != nil {
		return nil, fmt.Errorf("l1 block info from bytes: %v", err)
	}
	l1Attributes := &rolluptypes.MsgSetL1Attributes{
		L1BlockInfo: &rolluptypes.L1BlockInfo{
			Number:            l1BlockInfo.Number,
			Time:              l1BlockInfo.Time,
			BlockHash:         l1BlockInfo.BlockHash[:],
			SequenceNumber:    l1BlockInfo.SequenceNumber,
			BatcherAddr:       l1BlockInfo.BatcherAddr[:],
			L1FeeOverhead:     l1BlockInfo.L1FeeOverhead[:],
			L1FeeScalar:       l1BlockInfo.L1FeeScalar[:],
			BaseFeeScalar:     l1BlockInfo.BaseFeeScalar,
			BlobBaseFeeScalar: l1BlockInfo.BlobBaseFeeScalar,
		},
		EthTx: ethDepositTxs[0],
	}
	if l1Attributes.L1BlockInfo.BaseFee != nil {
		l1Attributes.L1BlockInfo.BaseFee = l1BlockInfo.BaseFee.Bytes()
	}
	if l1Attributes.L1BlockInfo.BlobBaseFee != nil {
		l1Attributes.L1BlockInfo.BlobBaseFee = l1BlockInfo.BlobBaseFee.Bytes()
	}

	depositTxs := make([]*rolluptypes.MsgApplyUserDeposit, 0)
	if len(ethDepositTxs) > 1 {
		for _, ethDepositTx := range ethDepositTxs[1:] {
			depositTxs = append(depositTxs, &rolluptypes.MsgApplyUserDeposit{
				Tx: ethDepositTx,
			})
		}
	}
	return &rolluptypes.DepositsTx{
		L1Attributes: l1Attributes,
		UserDeposits: depositTxs,
	}, nil
}

func convertToCosmosNonDepositTxs(nonDepositTxs []hexutil.Bytes) (bfttypes.Txs, error) {
	// Unpack Cosmos txs from ethTxs.
	cosmosTxs := make(bfttypes.Txs, 0, len(nonDepositTxs))
	for _, cosmosTx := range nonDepositTxs {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(cosmosTx); err != nil {
			return nil, fmt.Errorf("unmarshal binary tx: %v", err)
		}
		cosmosTxs = append(cosmosTxs, tx.Data())
	}

	return cosmosTxs, nil
}

func AdaptCosmosTxsToEthTxs(cosmosTxs bfttypes.Txs) (ethtypes.Transactions, error) {
	if cosmosTxs.Len() == 0 {
		return ethtypes.Transactions{}, nil
	}
	txsBytes := cosmosTxs.ToSliceOfBytes()
	ethTxs, err := GetDepositTxs(txsBytes[0])
	if err != nil {
		return nil, fmt.Errorf("get deposit txs: %v", err)
	}
	if len(txsBytes) > 1 {
		for _, txBytes := range txsBytes[1:] {
			ethTxs = append(ethTxs, AdaptNonDepositCosmosTxToEthTx(txBytes))
		}
	}

	return ethTxs, nil
}

func GetDepositTxs(cosmosDepositTx []byte) (ethtypes.Transactions, error) {
	msg := new(rolluptypes.DepositsTx)
	if err := msg.Unmarshal(cosmosDepositTx); err != nil {
		return nil, fmt.Errorf("unmarshal MsgL1Txs msg: %v", err)
	}
	var ethL1AttributesTx ethtypes.Transaction
	if err := ethL1AttributesTx.UnmarshalBinary(msg.L1Attributes.EthTx); err != nil {
		return nil, fmt.Errorf("unmarshal binary l1 attributes tx: %v", err)
	}
	txs := ethtypes.Transactions{&ethL1AttributesTx}

	ethTxsBytes := msg.GetUserDeposits()
	for _, userDepositTx := range ethTxsBytes {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(userDepositTx.Tx); err != nil {
			return nil, fmt.Errorf("unmarshal binary user deposit tx: %v", err)
		}
		if !tx.IsDepositTx() {
			return nil, errors.New("MsgL1Tx contains non-deposit tx")
		}
		txs = append(txs, &tx)
	}
	return txs, nil
}

func AdaptNonDepositCosmosTxToEthTx(cosmosTx bfttypes.Tx) *ethtypes.Transaction {
	return ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		// TODO maybe fill in other fields?
		Data: cosmosTx,
	})
}
