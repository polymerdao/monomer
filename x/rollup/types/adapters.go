package types

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	bfttypes "github.com/cometbft/cometbft/types"
	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
)

var errL1AttributesNotFound = errors.New("L1 attributes tx not found")

// AdaptPayloadTxsToCosmosTxs assumes the deposit transactions come first.
func AdaptPayloadTxsToCosmosTxs(ethTxs []hexutil.Bytes, appchainCtx cosmosclient.Context) (bfttypes.Txs, error) {
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
	msgAny, err := codectypes.NewAnyWithValue(&rollupv1.ApplyL1TxsRequest{
		TxBytes: depositTxsBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("new any with value: %v", err)
	}
	depositTx := sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: []*codectypes.Any{msgAny},
		},
	}

	privKey := ed25519.GenPrivKeyFromSecret([]byte("monomer"))
	pubKey := privKey.PubKey()
	address := pubKey.Address()

	txConfig := appchainCtx.TxConfig
	txBuilder := txConfig.NewTxBuilder()

	acc, err := appchainCtx.AccountRetriever.GetAccount(appchainCtx, sdktypes.AccAddress(address.Bytes()))

	if err != nil {
		return nil, fmt.Errorf("get account: %v", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       appchainCtx.ChainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
		PubKey:        pubKey,
		Address:       acc.GetAddress().String(),
	}

	blankSig := signing.SignatureV2{
		PubKey:   pubKey,
		Sequence: acc.GetSequence(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
	}

	err = txBuilder.SetMsgs(msgAny)
	if err != nil {
		return nil, fmt.Errorf("set msgs: %v", err)
	}
	err = txBuilder.SetSignatures(blankSig)
	if err != nil {
		return nil, fmt.Errorf("set signatures: %v", err)
	}

	sig, err := cosmostx.SignWithPrivKey(
		context.Background(),
		signing.SignMode_SIGN_MODE_DIRECT,
		signerData,
		txBuilder,
		privKey,
		txConfig,
		acc.GetSequence(),
	)
	if err != nil {
		return nil, fmt.Errorf("sign with priv key: %v", err)
	}
	err = txBuilder.SetSignatures(sig)
	if err != nil {
		return nil, fmt.Errorf("set signatures: %v", err)
	}
	// tx := txBuilder.GetTx()

	pubKeyAny, err := codectypes.NewAnyWithValue(pubKey)
	if err != nil {
		return nil, fmt.Errorf("new any with value: %v", err)
	}

	depositTx.AuthInfo = &sdktx.AuthInfo{
		SignerInfos: []*sdktx.SignerInfo{
			{
				PublicKey: pubKeyAny,
				ModeInfo: &sdktx.ModeInfo{
					// Assuming you want single signer mode
					Sum: &sdktx.ModeInfo_Single_{
						Single: &sdktx.ModeInfo_Single{
							Mode: signing.SignMode_SIGN_MODE_DIRECT,
						},
					},
				},
				Sequence: acc.GetSequence(),
			},
		},
	}
	depositTx.Signatures = [][]byte{sig.Data.(*signing.SingleSignatureData).Signature}

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
	msg := new(rollupv1.ApplyL1TxsRequest)
	if err := msg.Unmarshal(msgs[0].GetValue()); err != nil {
		return nil, fmt.Errorf("unmarshal MsgL1Txs smsg: %v", err)
	}
	ethTxsBytes := msg.GetTxBytes()
	if len(ethTxsBytes) == 0 {
		return nil, errors.New("L1 Attributes tx not found")
	}
	for _, txBytes := range ethTxsBytes {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			break
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
