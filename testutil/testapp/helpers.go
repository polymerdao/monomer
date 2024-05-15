package testapp

import (
	"encoding/json"
	"slices"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/polymerdao/monomer/gen/testapp/v1"
	"github.com/polymerdao/monomer/testutil/testapp/x/testmodule"
	"github.com/stretchr/testify/require"
)

const QueryPath = "/testapp.v1.GetService/Get"

func NewTest(t *testing.T, chainID string) *App {
	appdb := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, appdb.Close())
	})
	return New(appdb, chainID)
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

func ToTx(t *testing.T, k, v string) []byte {
	msgAny, err := codectypes.NewAnyWithValue(&testapp_v1.SetRequest{
		Key:   k,
		Value: v,
	})
	require.NoError(t, err)
	tx := &sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: []*codectypes.Any{msgAny},
		},
	}
	txBytes := make([]byte, tx.Size())
	_, err = tx.MarshalTo(txBytes)
	require.NoError(t, err)
	return txBytes
}

// ToTxs converts the key-values to SetRequest sdk.Msgs and marshals the messages to protobuf wire format.
// Each message is placed in a separate tx.
func ToTxs(t *testing.T, kvs map[string]string) [][]byte {
	var txs [][]byte
	for k, v := range kvs {
		txs = append(txs, ToTx(t, k, v))
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
		requestBytes, err := (&testapp_v1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp := a.Query(abcitypes.RequestQuery{
			Path:   QueryPath,
			Data:   requestBytes,
			Height: int64(height),
		})
		var val testapp_v1.GetResponse
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
		requestBytes, err := (&testapp_v1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp := a.Query(abcitypes.RequestQuery{
			Path:   "/testapp.v1.GetService/Get", // TODO is there a way to find this programmatically?
			Data:   requestBytes,
			Height: int64(height),
		})
		var val testapp_v1.GetResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		require.Empty(t, val.GetValue())
	}
}
