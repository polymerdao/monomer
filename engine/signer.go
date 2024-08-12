package engine

import (
	"context"
	"fmt"

	appchainClient "github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmoscrypto "github.com/cosmos/cosmos-sdk/crypto/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

type signer struct {
	appchainClient *appchainClient.Context
	privKey        *ed25519.PrivKey
	pubKey         *cryptotypes.PubKey
	address        *cosmoscrypto.Address
	bech32Address  *sdktypes.AccAddress
}

func NewSigner(appchainClient *appchainClient.Context, privKey *ed25519.PrivKey) *signer {
	return &signer{
		appchainClient: appchainClient,
		privKey:        privKey,
	}
}

func (s *signer) PubKey() *cryptotypes.PubKey {
	if s.pubKey != nil {
		return s.pubKey
	} else {
		pubKey := s.privKey.PubKey()
		s.pubKey = &pubKey
		return s.PubKey()
	}
}

func (s *signer) Address() cosmoscrypto.Address {
	if s.address != nil {
		return *s.address
	} else {
		address := s.privKey.PubKey().Address()
		s.address = &address
		return s.Address()
	}
}

func (s *signer) Bech32Addr() sdktypes.AccAddress {
	if s.bech32Address != nil {
		return *s.bech32Address
	} else {
		bech32Addr := sdktypes.AccAddress(s.Address())
		s.bech32Address = &bech32Addr
		return s.Bech32Addr()
	}
}

func (s *signer) Sign(tx *sdktx.Tx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during tx signing: %v", r)
		}
	}()

	txConfig := s.appchainClient.TxConfig
	txBuilder := txConfig.NewTxBuilder()

	acc, err := s.appchainClient.AccountRetriever.GetAccount(*s.appchainClient, s.Bech32Addr())
	if err != nil {
		return fmt.Errorf("get account: %v", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       s.appchainClient.ChainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
		PubKey:        *s.PubKey(),
		Address:       acc.GetAddress().String(),
	}

	blankSig := signing.SignatureV2{
		PubKey:   *s.PubKey(),
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
		s.privKey,
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

	pubKeyAny, err := codectypes.NewAnyWithValue(*s.PubKey())
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
		Fee: &sdktx.Fee{},
	}
	tx.Signatures = [][]byte{sig.Data.(*signing.SingleSignatureData).Signature}

	return nil
}
