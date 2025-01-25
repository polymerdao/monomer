package testapp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/gogoproto/proto"
	"github.com/polymerdao/monomer/testapp/x/testmodule"
	"github.com/polymerdao/monomer/testapp/x/testmodule/types"
	"github.com/stretchr/testify/require"
)

const QueryPath = "/testapp.v1.Query/Value"

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

func ToTestTx(t *testing.T, k, v string) []byte {
	// Generate a new private key for testing
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	addr := sdk.AccAddress(pubKey.Address())

	msg := &types.MsgSetValue{
		FromAddress: addr.String(),
		Key:         k,
		Value:       v,
	}

	return ToSignedTx(t, msg, privKey)
}

func ToSignedTx(t *testing.T, msg proto.Message, privKey cryptotypes.PrivKey) []byte {
	msgAny, err := codectypes.NewAnyWithValue(msg)
	require.NoError(t, err)

	// Create signed transaction
	tx := &sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: []*codectypes.Any{msgAny},
			Memo:     "",
		},
		AuthInfo: &sdktx.AuthInfo{
			Fee: &sdktx.Fee{
				Amount:   sdk.NewCoins(sdk.NewCoin("stake", sdk.NewInt(1000))),
				GasLimit: 200000,
			},
			SignerInfos: []*sdktx.SignerInfo{
				{
					PublicKey: codectypes.UnsafePackAny(privKey.PubKey()),
					ModeInfo: &sdktx.ModeInfo{
						Sum: &sdktx.ModeInfo_Single_{
							Single: &sdktx.ModeInfo_Single{
								Mode: signingtypes.SignMode_SIGN_MODE_DIRECT,
							},
						},
					},
					Sequence: 0,
				},
			},
		},
	}

	txBytes, err := tx.Marshal()
	require.NoError(t, err)
	return txBytes
}

// ToTxs converts the key-values to MsgSetValue sdk.Msgs and marshals the messages to protobuf wire format.
// Each message is placed in a separate tx.
func ToTxs(t *testing.T, kvs map[string]string) [][]byte {
	var txs [][]byte
	for k, v := range kvs {
		txs = append(txs, ToTestTx(t, k, v))
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
		requestBytes, err := (&types.QueryValueRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp, err := a.Query(context.Background(), &abcitypes.RequestQuery{
			Path:   QueryPath,
			Data:   requestBytes,
			Height: int64(height),
		})
		require.NoError(t, err)
		var val types.QueryValueResponse
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
		requestBytes, err := (&types.QueryValueRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp, err := a.Query(context.Background(), &abcitypes.RequestQuery{
			Path:   QueryPath,
			Data:   requestBytes,
			Height: int64(height),
		})
		require.NoError(t, err)
		var val types.QueryValueResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		require.Empty(t, val.GetValue())
	}
}
