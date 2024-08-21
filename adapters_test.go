package monomer_test

import (
	"testing"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/kataras/iris/v12/x/errors"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

func TestAdaptPayloadTxsToCosmosTxs(t *testing.T) {
	t.Run("nil tx", func(t *testing.T) {
		txs, err := monomer.AdaptPayloadTxsToCosmosTxs(nil, nil, "")
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("0 txs", func(t *testing.T) {
		txs, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{}, nil, "")
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	simpleSigner := func(_ *sdktx.Tx) error {
		return nil
	}

	tests := []struct {
		name, from            string
		depTxNum, nonDepTxNum int
		signTx                monomer.TxSigner
	}{
		{
			name:     "1 deposit tx",
			depTxNum: 1,
		},
		{
			name:        "1 deposit, 1 non-deposit txs",
			depTxNum:    1,
			nonDepTxNum: 1,
		},
		{
			name:        "10 deposit, 10 non-deposit  txs",
			depTxNum:    10,
			nonDepTxNum: 10,
		},
		{
			name:        "3 deposit, 3 non-deposit txs + from",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
		},
		{
			name:        "3 deposit, 3 non-deposit txs + signTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			signTx:      simpleSigner,
		},
		{
			name:        "3 deposit, 3 non-deposit txs, from address, signTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
			signTx:      simpleSigner,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			txBytes, _, hexTxBytes := generateTxsAndTxsBytes(t, test.depTxNum, test.nonDepTxNum)
			txs, err := monomer.AdaptPayloadTxsToCosmosTxs(hexTxBytes, test.signTx, test.from)
			require.NoError(t, err)

			msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{
				TxBytes:     txBytes[:test.depTxNum],
				FromAddress: test.from,
			})
			require.NoError(t, err)

			sdkTx := sdktx.Tx{
				Body: &sdktx.TxBody{
					Messages: []*codectypes.Any{msgAny},
				},
			}

			if test.signTx != nil {
				err := test.signTx(&sdkTx)
				require.NoError(t, err)
			}

			depositSDKMsgBytes, err := sdkTx.Marshal()
			require.NoError(t, err)

			cosmosTxs := make(bfttypes.Txs, 0, 1+test.nonDepTxNum)
			cosmosTxs = append(cosmosTxs, depositSDKMsgBytes)

			for _, cosmosTx := range txBytes[test.depTxNum:] {
				var tx ethtypes.Transaction
				err := tx.UnmarshalBinary(cosmosTx)
				require.NoError(t, err)
				cosmosTxs = append(cosmosTxs, tx.Data())
			}

			require.Equal(t, cosmosTxs, txs)
		})
	}

	t.Run("invalid attributes transaction", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{hexutil.Bytes("invalid")}, nil, "")
		require.Error(t, err)
	})

	t.Run("L1 attributes tx not found error", func(t *testing.T) {
		_, _, cosmosEthTx := testutils.GenerateEthTxs(t)
		cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
		require.NoError(t, err)
		_, err = monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{cosmosEthTxBytes}, nil, "")
		require.Error(t, err)
	})

	t.Run("sign tx error", func(t *testing.T) {
		_, depositTx, _ := testutils.GenerateEthTxs(t)
		depositTxBytes, err := depositTx.MarshalBinary()
		require.NoError(t, err)

		_, err = monomer.AdaptPayloadTxsToCosmosTxs(
			[]hexutil.Bytes{hexutil.Bytes(depositTxBytes)},
			func(tx *sdktx.Tx) error {
				return errors.New("sign tx error")
			},
			"")
		require.Error(t, err)
	})

	t.Run("unmarshal binary tx:", func(t *testing.T) {
		_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
		depositTxBytes, err := depositTx.MarshalBinary()
		require.NoError(t, err)
		cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
		require.NoError(t, err)

		_, err = monomer.AdaptPayloadTxsToCosmosTxs(
			[]hexutil.Bytes{
				hexutil.Bytes(depositTxBytes),
				hexutil.Bytes(cosmosEthTxBytes),
				hexutil.Bytes("invalid"),
			},
			nil,
			"")
		require.Error(t, err)
	})
}

func TestAdaptCosmosTxsToEthTxs(t *testing.T) { // Assume that AdaptPayloadTxsToCosmosTxs is correct
	t.Run("nil tx", func(t *testing.T) {
		txs, err := monomer.AdaptCosmosTxsToEthTxs(nil)
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("0 txd", func(t *testing.T) {
		txs, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{})
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	simpleSigner := func(_ *sdktx.Tx) error {
		return nil
	}

	tests := []struct {
		name, from            string
		depTxNum, nonDepTxNum int
		signTx                monomer.TxSigner
	}{
		{
			name:     "1 deposit tx",
			depTxNum: 1,
		},
		{
			name:        "1 deposit, 1 non-deposit txs",
			depTxNum:    1,
			nonDepTxNum: 1,
		},
		{
			name:        "10 deposit, 10 non-deposit  txs",
			depTxNum:    10,
			nonDepTxNum: 10,
		},
		{
			name:        "3 deposit, 3 non-deposit txs + from",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
		},
		{
			name:        "3 deposit, 3 non-deposit txs + signTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			signTx:      simpleSigner,
		},
		{
			name:        "3 deposit, 3 non-deposit txs, from address, signTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
			signTx:      simpleSigner,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, ethTxs, txBytes := generateTxsAndTxsBytes(t, test.depTxNum, test.nonDepTxNum)
			txs, err := monomer.AdaptPayloadTxsToCosmosTxs(txBytes, test.signTx, test.from)
			require.NoError(t, err)

			adaptedTxs, err := monomer.AdaptCosmosTxsToEthTxs(txs)
			require.NoError(t, err)
			require.Equal(t, len(ethTxs), len(adaptedTxs))
			for i := range ethTxs {
				require.Equal(t, ethTxs[i].Hash(), adaptedTxs[i].Hash(), i)
			}
		})
	}

	t.Run("unmarshal cosmos tx error", func(t *testing.T) {
		_, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{[]byte("invalid")})
		require.Error(t, err)
	})

	t.Run("unexpected number of msgs in Eth Cosmos tx error", func(t *testing.T) {
		sdkTx := sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{},
			},
		}

		depositSDKMsgBytes, err := sdkTx.Marshal()
		require.NoError(t, err)

		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("L1 Attributes tx not found error", func(t *testing.T) {
		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{})
		require.NoError(t, err)

		sdkTx := sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{msgAny},
			},
		}

		depositSDKMsgBytes, err := sdkTx.Marshal()
		require.NoError(t, err)

		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("unmarshal MsgL1Txs smsg error", func(t *testing.T) {
		sdkTx := sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{
					{
						Value: []byte("invalid"),
					},
				},
			},
		}

		depositSDKMsgBytes, err := sdkTx.Marshal()
		require.NoError(t, err)

		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("MsgL1Tx contains non-deposit tx error", func(t *testing.T) {
		nonDepNum := 5
		txBytes, _, _ := generateTxsAndTxsBytes(t, 0, nonDepNum)

		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{
			TxBytes: txBytes,
		})
		require.NoError(t, err)

		sdkTx := sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{msgAny},
			},
		}

		depositSDKMsgBytes, err := sdkTx.Marshal()
		require.NoError(t, err)

		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("unmarshal binary error", func(t *testing.T) {
		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{
			TxBytes: [][]byte{[]byte("invalid")},
		})
		require.NoError(t, err)

		sdkTx := sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{msgAny},
			},
		}

		depositSDKMsgBytes, err := sdkTx.Marshal()
		require.NoError(t, err)

		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})
}

func generateTxsAndTxsBytes(
	t *testing.T,
	depTxNum int,
	nonDepTxNum int,
) ([][]byte, ethtypes.Transactions, []hexutil.Bytes) {
	_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	txBytes := make([][]byte, 0, depTxNum+nonDepTxNum)
	txs := make(ethtypes.Transactions, 0, depTxNum+nonDepTxNum)

	depositTxBytes, err := depositTx.MarshalBinary()
	require.NoError(t, err)
	for range depTxNum {
		txBytes = append(txBytes, depositTxBytes)
		txs = append(txs, depositTx)
	}

	cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
	require.NoError(t, err)
	for range nonDepTxNum {
		txBytes = append(txBytes, cosmosEthTxBytes)
		txs = append(txs, cosmosEthTx)
	}

	hexTxBytes := make([]hexutil.Bytes, 0, depTxNum+nonDepTxNum)
	for _, tx := range txBytes {
		hexTxBytes = append(hexTxBytes, hexutil.Bytes(tx))
	}
	return txBytes, txs, hexTxBytes
}
