package peptide

import (
	"github.com/cosmos/cosmos-sdk/client"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsign "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"math/rand"
	"time"
)

const (
	DefaultGenTxGas = 200000000
)

// GenTx generates a signed mock transaction.
// func GenTx(gen client.TxConfig, msgs []sdk.Msg, feeAmt sdk.Coins, gas uint64, chainID string, accNums, accSeqs []uint64, priv ...cryptotypes.PrivKey) (sdk.Tx, error) {
func GenTx(
	gen client.TxConfig,
	msgs []sdk.Msg,
	feeAmt sdk.Coins,
	gas uint64,
	chainID string,
	r *rand.Rand,
	signerAccounts ...*SignerAccount,
) (sdk.Tx, error) {
	sigs := make([]signing.SignatureV2, len(signerAccounts))

	// create a random length memo
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	memo := simulation.RandStringOfLength(r, simulation.RandIntBetween(r, 0, 100))

	signMode := gen.SignModeHandler().DefaultMode()

	// 1st round: set SignatureV2 with empty signatures, to set correct
	// signer infos.
	for i, signer := range signerAccounts {
		sigs[i] = signing.SignatureV2{
			PubKey: signer.PubKey(),
			Data: &signing.SingleSignatureData{
				SignMode: signMode,
			},
			Sequence: signer.GetSequence(),
		}
	}

	tx := gen.NewTxBuilder()
	err := tx.SetMsgs(msgs...)
	if err != nil {
		return nil, err
	}
	err = tx.SetSignatures(sigs...)
	if err != nil {
		return nil, err
	}
	tx.SetMemo(memo)
	tx.SetFeeAmount(feeAmt)
	tx.SetGasLimit(gas)

	// 2nd round: once all signer infos are set, every signer can sign.
	for i, p := range signerAccounts {
		signerData := authsign.SignerData{
			ChainID:       chainID,
			AccountNumber: p.GetAccountNumber(),
			Sequence:      p.GetSequence(),
		}
		signBytes, err := gen.SignModeHandler().GetSignBytes(signMode, signerData, tx.GetTx())
		if err != nil {
			panic(err)
		}
		sig, err := p.Sign(signBytes)
		if err != nil {
			panic(err)
		}
		sigs[i].Data.(*signing.SingleSignatureData).Signature = sig
		err = tx.SetSignatures(sigs...)
		if err != nil {
			panic(err)
		}
	}

	return tx.GetTx(), nil
}

// TxSigner represents a group of signers that can sign a transaction.
// all tx related configs can be cached and can be changed when needed.
type TxSigner struct {
	// txConfig is the tx config used to generate tx
	txConfig client.TxConfig
	feeAmt   sdk.Coins
	gas      uint64
	chainID  string
	accNums  []uint64
	accSeqs  []uint64
	priv     []cryptotypes.PrivKey
}

// NewTxSigner creates a new TxSigner.
func NewTxSigner(
	txConfig client.TxConfig,
	feeAmt sdk.Coins,
	gas uint64,
	chainID string,
) TxSigner {

	return TxSigner{
		txConfig: txConfig,
		feeAmt:   feeAmt,
		gas:      gas,
		chainID:  chainID,
	}
}

// AddSinger adds a signer to the TxSigner.
func (s *TxSigner) AddSigner(accNum, accSeq uint64, priv cryptotypes.PrivKey) {
	s.accNums = append(s.accNums, accNum)
	s.accSeqs = append(s.accSeqs, accSeq)
	s.priv = append(s.priv, priv)
}

// SignTx signs a transaction.
func (s TxSigner) SignTx(memo string, msgs ...sdk.Msg) (sdk.Tx, error) {
	sigs := make([]signing.SignatureV2, len(s.priv))

	signMode := s.txConfig.SignModeHandler().DefaultMode()

	// 1st round: set SignatureV2 with empty signatures, to set correct
	// signer infos.
	for i, signer := range s.priv {
		sigs[i] = signing.SignatureV2{
			PubKey: signer.PubKey(),
			Data: &signing.SingleSignatureData{
				SignMode: signMode,
			},
			Sequence: s.accSeqs[i],
		}
	}

	tx := s.txConfig.NewTxBuilder()
	err := tx.SetMsgs(msgs...)
	if err != nil {
		return nil, err
	}
	err = tx.SetSignatures(sigs...)
	if err != nil {
		return nil, err
	}
	tx.SetMemo(memo)
	tx.SetFeeAmount(s.feeAmt)
	tx.SetGasLimit(s.gas)

	// 2nd round: once all signer infos are set, every signer can sign.
	for i, p := range s.priv {
		signerData := authsign.SignerData{
			ChainID:       s.chainID,
			AccountNumber: s.accNums[i],
			Sequence:      s.accSeqs[i],
		}
		signBytes, err := s.txConfig.SignModeHandler().GetSignBytes(signMode, signerData, tx.GetTx())
		if err != nil {
			panic(err)
		}
		sig, err := p.Sign(signBytes)
		if err != nil {
			panic(err)
		}
		sigs[i].Data.(*signing.SingleSignatureData).Signature = sig
		err = tx.SetSignatures(sigs...)
		if err != nil {
			panic(err)
		}
	}

	return tx.GetTx(), nil
}
