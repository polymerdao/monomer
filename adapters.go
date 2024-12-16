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

// AdaptPayloadTxsToCosmosTxs assumes the deposit transactions come first.
func AdaptPayloadTxsToCosmosTxs(ethTxs []hexutil.Bytes) (bfttypes.Txs, error) {
	// L1 Attributes transaction.
	if len(ethTxs) == 0 {
		return nil, errors.New("l1 attributes transaction not found")
	}
	l1AttributesTxBytes := ethTxs[0]
	ethTxs = ethTxs[1:]
	var ethL1AttributesTx ethtypes.Transaction
	if err := ethL1AttributesTx.UnmarshalBinary(l1AttributesTxBytes); err != nil {
		return nil, fmt.Errorf("unmarshal binary: %v", err)
	}
	if ethL1AttributesTx.Type() != ethtypes.DepositTxType {
		return nil, errors.New("first transaction is not a deposit transaction")
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
		EthTx: l1AttributesTxBytes,
	}
	if l1BlockInfo.BaseFee != nil {
		l1Attributes.L1BlockInfo.BaseFee = l1BlockInfo.BaseFee.Bytes()
	}
	if l1BlockInfo.BlobBaseFee != nil {
		l1Attributes.L1BlockInfo.BlobBaseFee = l1BlockInfo.BlobBaseFee.Bytes()
	}

	// User deposit transactions.
	depositTxs := make([]*rolluptypes.MsgApplyUserDeposit, 0)
	for _, ethDepositTxBytes := range ethTxs {
		var ethDepositTx ethtypes.Transaction
		if err := ethDepositTx.UnmarshalBinary(ethDepositTxBytes); err != nil {
			return nil, fmt.Errorf("unmarshal binary eth deposit tx bytes: %v", err)
		}
		if !ethDepositTx.IsDepositTx() {
			break // We have reached the end of the deposit txs.
		}
		depositTxs = append(depositTxs, &rolluptypes.MsgApplyUserDeposit{
			Tx: ethDepositTxBytes,
		})
	}
	ethTxs = ethTxs[len(depositTxs):]

	cosmosTxs := make(bfttypes.Txs, 0, 1+len(ethTxs))

	depositTxBytes, err := (&rolluptypes.DepositsTx{
		L1Attributes: l1Attributes,
		UserDeposits: depositTxs,
	}).Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal tx: %v", err)
	}
	cosmosTxs = append(cosmosTxs, depositTxBytes)

	// Non-deposit transactions.
	for _, cosmosTx := range ethTxs {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(cosmosTx); err != nil {
			return nil, fmt.Errorf("unmarshal binary tx: %v", err)
		}
		if tx.IsDepositTx() {
			return nil, errors.New("found a deposit tx after a non-deposit tx")
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
	cosmosDepositTxBytes := txsBytes[0]
	txsBytes = txsBytes[1:]

	cosmosDepositTx := new(rolluptypes.DepositsTx)
	if err := cosmosDepositTx.Unmarshal(cosmosDepositTxBytes); err != nil {
		return nil, fmt.Errorf("unmarshal MsgL1Txs msg: %v", err)
	}

	// L1 Attributes.
	var ethL1AttributesTx ethtypes.Transaction
	if err := ethL1AttributesTx.UnmarshalBinary(cosmosDepositTx.L1Attributes.EthTx); err != nil {
		return nil, fmt.Errorf("unmarshal binary l1 attributes tx: %v", err)
	}

	ethTxs := ethtypes.Transactions{&ethL1AttributesTx}

	// User Deposits.
	cosmosUserDepositTxs := cosmosDepositTx.GetUserDeposits()
	for _, cosmosUserDepositTx := range cosmosUserDepositTxs {
		var ethUserDepositTx ethtypes.Transaction
		if err := ethUserDepositTx.UnmarshalBinary(cosmosUserDepositTx.Tx); err != nil {
			return nil, fmt.Errorf("unmarshal binary user deposit tx: %v", err)
		}
		if !ethUserDepositTx.IsDepositTx() {
			return nil, errors.New("cosmos deposit tx contains non-deposit tx")
		}
		ethTxs = append(ethTxs, &ethUserDepositTx)
	}

	// Non-deposit transactions.
	for _, txBytes := range txsBytes {
		ethTxs = append(ethTxs, AdaptNonDepositCosmosTxToEthTx(txBytes))
	}

	return ethTxs, nil
}

func AdaptNonDepositCosmosTxToEthTx(cosmosTx bfttypes.Tx) *ethtypes.Transaction {
	return ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		// TODO maybe fill in other fields?
		Data: cosmosTx,
	})
}
