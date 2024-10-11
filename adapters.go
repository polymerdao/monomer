package monomer

import (
	"errors"
	"fmt"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

var PrivKey = secp256k1.GenPrivKeyFromSecret([]byte("monomer"))

var errL1AttributesNotFound = errors.New("L1 attributes tx not found")

type TxSigner func([]sdktypes.Msg) (bfttypes.Tx, error)

// AdaptPayloadTxsToCosmosTxs assumes the deposit transactions come first.
func AdaptPayloadTxsToCosmosTxs(ethTxs []hexutil.Bytes, _ TxSigner, _ string) (bfttypes.Txs, error) {
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

	cosmosTxs := make(bfttypes.Txs, 0, 1+numDepositTxs)
	cosmosTxs = append(cosmosTxs, depositTxBytes)

	cosmosNonDepositTxs, err := convertToCosmosNonDepositTxs(ethTxs[numDepositTxs:])
	if err != nil {
		return nil, fmt.Errorf("convert to cosmos txs: %v", err)
	}

	cosmosTxs = append(cosmosTxs, cosmosNonDepositTxs...)

	return cosmosTxs, nil
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

func packDepositTxsToCosmosTx(depositTxs []hexutil.Bytes, _ string) (*rolluptypes.MsgApplyL1Txs, error) {
	depositTxsBytes := make([][]byte, 0, len(depositTxs))
	for _, depositTx := range depositTxs {
		depositTxsBytes = append(depositTxsBytes, depositTx)
	}
	return &rolluptypes.MsgApplyL1Txs{
		TxBytes: depositTxsBytes,
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
	ethTxs, err := GetDepositTxs(txsBytes)
	if err != nil {
		return nil, fmt.Errorf("get deposit txs: %v", err)
	}
	for _, txBytes := range txsBytes[1:] {
		ethTxs = append(ethTxs, AdaptNonDepositCosmosTxToEthTx(txBytes))
	}

	return ethTxs, nil
}

func GetDepositTxs(txsBytes [][]byte) (ethtypes.Transactions, error) {
	cosmosEthTx := new(sdktx.Tx)
	if err := cosmosEthTx.Unmarshal(txsBytes[0]); err != nil {
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
	txs := make(ethtypes.Transactions, 0, len(ethTxsBytes)+len(txsBytes)-1)
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
	return txs, nil
}

func AdaptNonDepositCosmosTxToEthTx(cosmosTx bfttypes.Tx) *ethtypes.Transaction {
	return ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		// TODO maybe fill in other fields?
		Data: cosmosTx,
	})
}
