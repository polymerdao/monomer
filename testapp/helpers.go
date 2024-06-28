package testapp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
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

func ToTx(t *testing.T, k, v string, sk *secp256k1.PrivKey, pk *secp256k1.PubKey) []byte {
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
		ChainID:       "test",
		AccountNumber: 0,
		Sequence:      0,
		PubKey:        pk,
	}

	txBuilder.SetGasLimit(100000)
	txBuilder.SetFeePayer(fromAddr)

	sig, err := tx.SignWithPrivKey(context.TODO(), signing.SignMode_SIGN_MODE_DIRECT, signerData, txBuilder, sk, txConfig, 0)
	require.NoError(t, err)
	err = txBuilder.SetSignatures(sig)
	require.NoError(t, err)

	txBytes, err := txConfig.TxEncoder()(txBuilder.GetTx())
	require.NoError(t, err)
	return txBytes
}

// ToTxs converts the key-values to SetRequest sdk.Msgs and marshals the messages to protobuf wire format.
// Each message is placed in a separate tx.
func ToTxs(t *testing.T, kvs map[string]string, sk *secp256k1.PrivKey, pk *secp256k1.PubKey) [][]byte {
	var txs [][]byte
	for k, v := range kvs {
		txs = append(txs, ToTx(t, k, v, sk, pk))
	}
	// Ensure txs are always returned in the same order.
	slices.SortFunc(txs, func(x []byte, y []byte) int {
		stringX := string(x)
		stringY := string(y)
		if stringX > stringY {
			return 1
		} else if stringX < stringY {
			return -1
		}
		return 0
	})
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

func TestAccount() (*secp256k1.PrivKey, *secp256k1.PubKey) {
    sk := secp256k1.GenPrivKey()
    pk := secp256k1.PubKey{
        Key: sk.PubKey().Bytes(),
    }
    return sk, &pk
}
