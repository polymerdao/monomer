package signer

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	bfttypes "github.com/cometbft/cometbft/types"
	appchainClient "github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/gogoproto/proto"
	"github.com/polymerdao/monomer/x/rollup/types"
)

type Signer struct {
	appchainCtx    *appchainClient.Context
	privKey        *secp256k1.PrivKey
	pubKey         *cryptotypes.PubKey
	accountAddress *sdktypes.AccAddress
}

func New(appchainCtx *appchainClient.Context, privKey *secp256k1.PrivKey) *Signer {
	pubKey := privKey.PubKey()
	accAddress := sdktypes.AccAddress(pubKey.Address())

	return &Signer{
		appchainCtx:    appchainCtx,
		privKey:        privKey,
		pubKey:         &pubKey,
		accountAddress: &accAddress,
	}
}

func (s *Signer) PubKey() cryptotypes.PubKey {
	if s.pubKey == nil {
		pubKey := s.privKey.PubKey()
		s.pubKey = &pubKey
	}
	return *s.pubKey
}

func (s *Signer) AccountAddress() sdktypes.AccAddress {
	if s.accountAddress == nil {
		accAddress := sdktypes.AccAddress(s.PubKey().Address())
		s.accountAddress = &accAddress
	}
	return *s.accountAddress
}

func (s *Signer) Sign(msgs []proto.Message) (_ bfttypes.Tx, err error) {
	acc, err := s.appchainCtx.AccountRetriever.GetAccount(*s.appchainCtx, s.AccountAddress())
	if err != nil {
		return nil, fmt.Errorf("get account: %v", err)
	}

	txConfig := s.appchainCtx.TxConfig

	txBuilder := txConfig.NewTxBuilder()
	if err := txBuilder.SetMsgs(msgs...); err != nil {
		return nil, fmt.Errorf("set msgs: %v", err)
	}
	txBuilder.SetFeeAmount(sdktypes.NewCoins(sdktypes.NewCoin(types.ETH, sdkmath.NewInt(1000000))))
	txBuilder.SetGasLimit(1000000)
	if err := txBuilder.SetSignatures(signing.SignatureV2{ // TODO
		PubKey:   acc.GetPubKey(),
		Sequence: acc.GetSequence(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
	}); err != nil {
		return nil, fmt.Errorf("set blank signature: %v", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       s.appchainCtx.ChainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
		PubKey:        acc.GetPubKey(),
		Address:       acc.GetAddress().String(),
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
		return nil, fmt.Errorf("sign with priv key: %v", err)
	}
	if err = txBuilder.SetSignatures(sig); err != nil {
		return nil, fmt.Errorf("set signatures: %v", err)
	}

	tx := txBuilder.GetTx()
	txBytes, err := txConfig.TxEncoder()(tx)
	if err != nil {
		return nil, fmt.Errorf("encode tx: %v", err)
	}

	return txBytes, nil
}
