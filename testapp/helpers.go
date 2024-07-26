package testapp

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/polymerdao/monomer/gen/testapp/v1"
	"github.com/polymerdao/monomer/testapp/x/testmodule"
	"github.com/stretchr/testify/require"
)

const QueryPath = "/testapp.v1.GetService/Get"

func NewTest(t *testing.T, chainID string) *App {
	appdb := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, appdb.Close())
	})
	app, err := New(appdb, chainID)
	require.NoError(t, err)
	return app
}

func MakeGenesisAppState(t *testing.T, app *App, kvs ...string) map[string]json.RawMessage {
	require.True(t, len(kvs)%2 == 0)
	defaultGenesis := app.DefaultGenesis()
	kvsMap := make(map[string]string)
	require.NoError(t, json.Unmarshal(defaultGenesis[testmodule.ModuleName], &kvsMap))
	{
		var k string
		for i, x := range kvs {
			if i%2 == 0 {
				k = x
			} else {
				kvsMap[k] = x
			}
		}
	}
	kvsMapBytes, err := json.Marshal(kvsMap)
	require.NoError(t, err)
	defaultGenesis[testmodule.ModuleName] = kvsMapBytes
	return defaultGenesis
}

// ToTx converts the key-value to a testappv1.SetRequest message and returns the encoded transaction bytes. It also handles
// signing the transaction with the provided private key. The caller is responsible for ensuring the account sequence is
// correct.
func ToTx(t *testing.T, k, v, chainID string, sk *secp256k1.PrivKey, acc sdk.AccountI, seq uint64, ctx sdk.Context) []byte {
	pk := sk.PubKey()
	fromAddr := sdk.AccAddress(pk.Address())

	msg := &testappv1.SetRequest{
		FromAddress: fromAddr.String(),
		Key:         k,
		Value:       v,
	}

	txConfig := testutil.MakeTestTxConfig()
	txBuilder := txConfig.NewTxBuilder()
	err := txBuilder.SetMsgs(msg)
	require.NoError(t, err)

	signerData := authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      seq,
		PubKey:        pk,
		Address:       pk.Address().String(),
	}

	txBuilder.SetGasLimit(100000)
	txBuilder.SetFeePayer(fromAddr)

	emptySig := signing.SignatureV2{
		PubKey: pk,
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
		Sequence: seq,
	}
	err = txBuilder.SetSignatures(emptySig)
	require.NoError(t, err)

	sig, err := tx.SignWithPrivKey(ctx, signing.SignMode_SIGN_MODE_DIRECT, signerData, txBuilder, sk, txConfig, seq)
	require.NoError(t, err)
	err = txBuilder.SetSignatures(sig)
	require.NoError(t, err)

	tx := txBuilder.GetTx()
	_, err = tx.GetSigners()
	require.NoError(t, err)

	signedTx := txBuilder.GetTx()
	txBytes, err := txConfig.TxEncoder()(signedTx)
	require.NoError(t, err)

	return txBytes
}

// ToTxs converts the key-values to SetRequest sdk.Msgs and marshals the messages to protobuf wire format.
// Each message is placed in a separate tx.
func ToTxs(t *testing.T, kvs map[string]string, chainID string, sk *secp256k1.PrivKey, acc sdk.AccountI, startingSeq uint64, ctx sdk.Context) [][]byte {
	// Extract the keys and sort them
	keys := make([]string, 0, len(kvs))
	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var txs [][]byte
	for _, k := range keys {
		v := kvs[k]
		txs = append(txs, ToTx(t, k, v, chainID, sk, acc, startingSeq, ctx))
		startingSeq += 1
	}
	return txs
}

// StateContains ensures the key-values exist in the testmodule's state.
func (a *App) StateContains(t *testing.T, height uint64, kvs map[string]string) {
	if len(kvs) == 0 {
		return
	}
	gotState := make(map[string]string, len(kvs))
	for k := range kvs {
		requestBytes, err := (&testappv1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp, err := a.Query(context.Background(), &abcitypes.RequestQuery{
			Path:   QueryPath,
			Data:   requestBytes,
			Height: int64(height),
		})
		require.NoError(t, err)
		var val testappv1.GetResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		gotState[k] = val.GetValue()
	}
	require.Equal(t, kvs, gotState)
}

// StateDoesNotContain ensures the key-values do not exist in the app's state.
func (a *App) StateDoesNotContain(t *testing.T, height uint64, kvs map[string]string) {
	if len(kvs) == 0 {
		return
	}
	for k := range kvs {
		requestBytes, err := (&testappv1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp, err := a.Query(context.Background(), &abcitypes.RequestQuery{
			Path:   "/testapp.v1.GetService/Get", // TODO is there a way to find this programmatically?
			Data:   requestBytes,
			Height: int64(height),
		})
		require.NoError(t, err)
		var val testappv1.GetResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		require.Empty(t, val.GetValue())
	}
}

// TestAccount loads a pre-funded test account with a private key from the testdata directory.
func (a *App) TestAccount(ctx sdk.Context) (*secp256k1.PrivKey, *secp256k1.PubKey, sdk.AccountI) {
	// The private key corresponding to our pre-funded test account
	skBytes := []byte{85, 55, 6, 26, 146, 55, 109, 224, 90, 200, 239, 207, 63, 193, 137, 45, 143, 86, 124, 111, 39, 66, 173, 26, 133, 208, 231, 109, 66, 224, 101, 79}
	sk := secp256k1.PrivKey{
		Key: skBytes,
	}
	pk := secp256k1.PubKey{
		Key: sk.PubKey().Bytes(),
	}

	// Set the account with the testapp's account keeper
	accAddr := sdk.AccAddress(pk.Address())
	// TODO: debug why this sometimes sets the account number to 4
	account := a.accountKeeper.NewAccountWithAddress(ctx, accAddr)
	a.accountKeeper.SetAccount(ctx, account)

	err := account.SetPubKey(sk.PubKey())
	if err != nil {
		panic("Failed to set account public key: %v" + err.Error())
	}

	err = account.SetAccountNumber(0)
	if err != nil {
		panic("Failed to set account number: %v" + err.Error())
	}

	// This is required for signing transactions. If we do not set the params, the signature limit will be 0.
	// Default params will set the signature limit to 7.
	a.accountKeeper.Params.Set(ctx, authtypes.DefaultParams())

	return &sk, &pk, account
}
