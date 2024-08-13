package engine

import (
	"context"
	"fmt"

	appchainClient "github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

type signer struct {
	appchainCtx    *appchainClient.Context
	privKey        *ed25519.PrivKey
	pubKey         *cryptotypes.PubKey
	accountAddress *sdktypes.AccAddress
}

func NewSigner(appchainCtx *appchainClient.Context, privKey *ed25519.PrivKey) *signer {
	pubKey := privKey.PubKey()
	accAddress := sdktypes.AccAddress(pubKey.Address())

	return &signer{
		appchainCtx:    appchainCtx,
		privKey:        privKey,
		pubKey:         &pubKey,
		accountAddress: &accAddress,
	}
}

func (s *signer) PubKey() cryptotypes.PubKey {
	if s.pubKey == nil {
		pubKey := s.privKey.PubKey()
		s.pubKey = &pubKey
	}
	return *s.pubKey
}

func (s *signer) AccountAddress() sdktypes.AccAddress {
	if s.accountAddress == nil {
		accAddress := sdktypes.AccAddress(s.PubKey().Address())
		s.accountAddress = &accAddress
	}
	return *s.accountAddress
}

// Applies a signiture and related metadata to the provided transaction using the signer's private key.
func (s *signer) Sign(tx *sdktx.Tx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during tx signing: %v", r)
		}
	}()

	txConfig := s.appchainCtx.TxConfig
	txBuilder := txConfig.NewTxBuilder()

	acc, err := s.appchainCtx.AccountRetriever.GetAccount(*s.appchainCtx, s.AccountAddress())
	if err != nil {
		return fmt.Errorf("get account: %v", err)
	}

	seq := acc.GetSequence()

	signerData := authsigning.SignerData{
		ChainID:       s.appchainCtx.ChainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      seq,
		PubKey:        s.PubKey(),
		Address:       acc.GetAddress().String(),
	}

	blankSig := signing.SignatureV2{
		PubKey:   s.PubKey(),
		Sequence: acc.GetSequence(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
	}

	messages := make([]sdktypes.Msg, 0)
	for _, m := range tx.Body.Messages {
		messages = append(messages, m)
	}

	err = txBuilder.SetMsgs(messages...)
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
		s.privKey,
		txConfig,
		seq,
	)
	if err != nil {
		return fmt.Errorf("sign with priv key: %v", err)
	}
	err = txBuilder.SetSignatures(sig)
	if err != nil {
		return fmt.Errorf("set signatures: %v", err)
	}

	pubKeyAny, err := codectypes.NewAnyWithValue(s.PubKey())
	if err != nil {
		return fmt.Errorf("new any with value: %v", err)
	}

	tx.AuthInfo = &sdktx.AuthInfo{
		SignerInfos: []*sdktx.SignerInfo{
			{
				PublicKey: pubKeyAny,
				ModeInfo: &sdktx.ModeInfo{
					Sum: &sdktx.ModeInfo_Single_{
						Single: &sdktx.ModeInfo_Single{
							Mode: signing.SignMode_SIGN_MODE_DIRECT,
						},
					},
				},
				Sequence: seq,
			},
		},
		Fee: &sdktx.Fee{},
	}
	tx.Signatures = [][]byte{sig.Data.(*signing.SingleSignatureData).Signature}

	return nil
}
