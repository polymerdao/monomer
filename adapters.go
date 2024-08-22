package monomer

import (
	"errors"
	"fmt"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

var errL1AttributesNotFound = errors.New("L1 attributes tx not found")

type txSigner func(tx *sdktx.Tx) error

// AdaptPayloadTxsToCosmosTxs assumes the deposit transactions come first.
func AdaptPayloadTxsToCosmosTxs(ethTxs []hexutil.Bytes, signTx txSigner, from string) (bfttypes.Txs, error) {
	if len(ethTxs) == 0 {
		return bfttypes.Txs{}, nil
	}

	// Count number of deposit txs.
	var numDepositTxs int
	for _, txBytes := range ethTxs {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			return nil, fmt.Errorf("unmarshal binary: %v", err)
		}
		if tx.IsDepositTx() {
			numDepositTxs++
		} else {
			break // Assume deposit transactions must come first.
		}
	}
	if numDepositTxs == 0 {
		return nil, errL1AttributesNotFound
	}

	// Pack deposit txs into an SDK Msg.
	var depositTxsBytes [][]byte
	for _, depositTx := range ethTxs[:numDepositTxs] {
		depositTxsBytes = append(depositTxsBytes, depositTx)
	}
	msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{
		TxBytes:     depositTxsBytes,
		FromAddress: from,
	})
	if err != nil {
		return nil, fmt.Errorf("new any with value: %v", err)
	}
	depositTx := sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: []*codectypes.Any{msgAny},
		},
	}

	if signTx != nil {
		if err := signTx(&depositTx); err != nil {
			return nil, fmt.Errorf("sign tx: %v", err)
		}
	}

	depositSDKMsgBytes, err := depositTx.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal tx: %v", err)
	}

	// Unpack Cosmos txs from ethTxs.
	cosmosTxs := bfttypes.ToTxs([][]byte{depositSDKMsgBytes})
	for _, cosmosTx := range ethTxs[numDepositTxs:] {
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

	var txs ethtypes.Transactions

	// Unpack deposits from the MsgL1Txs msg.
	cosmosEthTxBytes := txsBytes[0]
	cosmosEthTx := new(sdktx.Tx)
	if err := cosmosEthTx.Unmarshal(cosmosEthTxBytes); err != nil {
		return nil, fmt.Errorf("unmarshal cosmos tx: %v", err)
	}
	msgs := cosmosEthTx.GetBody().GetMessages()
	if num := len(msgs); num != 1 {
		return nil, fmt.Errorf("unexpected number of msgs in Eth Cosmos tx: want 1, got %d", num)
	}
	msg := new(rolluptypes.MsgApplyL1Txs)
	if err := msg.Unmarshal(msgs[0].GetValue()); err != nil {
		return nil, fmt.Errorf("unmarshal MsgL1Txs smsg: %v", err)
	}
	ethTxsBytes := msg.GetTxBytes()
	if len(ethTxsBytes) == 0 {
		return nil, errL1AttributesNotFound
	}
	for _, txBytes := range ethTxsBytes {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			return nil, fmt.Errorf("unmarshal binary: %v", err)
		}
		if !tx.IsDepositTx() {
			return nil, errors.New("MsgL1Tx contains non-deposit tx")
		}
		txs = append(txs, &tx)
	}

	// Pack Cosmos txs into Ethereum txs.
	for _, txBytes := range txsBytes[1:] {
		txs = append(txs, AdaptNonDepositCosmosTxToEthTx(txBytes))
	}

	return txs, nil
}

func AdaptNonDepositCosmosTxToEthTx(cosmosTx bfttypes.Tx) *ethtypes.Transaction {
	return ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		// TODO maybe fill in other fields?
		Data: cosmosTx,
	})
}
