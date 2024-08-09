package engine

import (
	"context"
	"fmt"

	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

func (e *EngineAPI) signer(tx *sdktx.Tx) error {
	privKey := ed25519.GenPrivKeyFromSecret([]byte("monomer"))
	pubKey := privKey.PubKey()
	address := pubKey.Address()

	txConfig := e.appChainClient.TxConfig
	txBuilder := txConfig.NewTxBuilder()

	acc, err := e.appChainClient.AccountRetriever.GetAccount(*e.appChainClient, sdktypes.AccAddress(address.Bytes()))
	if err != nil {
		return fmt.Errorf("get account: %v", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       e.appChainClient.ChainID,
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

	err = txBuilder.SetMsgs(tx.Body.Messages[0])
	if err != nil {
		return fmt.Errorf("set msgs: %v", err)
	}
	err = txBuilder.SetSignatures(blankSig)
	if err != nil {
		return fmt.Errorf("set signatures: %v", err)
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
		return fmt.Errorf("sign with priv key: %v", err)
	}
	err = txBuilder.SetSignatures(sig)
	if err != nil {
		return fmt.Errorf("set signatures: %v", err)
	}

	pubKeyAny, err := codectypes.NewAnyWithValue(pubKey)
	if err != nil {
		return fmt.Errorf("new any with value: %v", err)
	}

	tx.AuthInfo = &sdktx.AuthInfo{
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
	tx.Signatures = [][]byte{sig.Data.(*signing.SingleSignatureData).Signature}

	return nil
	// }
}
