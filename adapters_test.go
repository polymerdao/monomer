package monomer_test

import (
	"errors"
	"testing"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

var tests = []struct {
	name, from            string
	depTxNum, nonDepTxNum int
	signTx                monomer.TxSigner
}{
	{
		name:     "correctly converts a single deposit tx without signer or from address",
		depTxNum: 1,
	},
	{
		name:        "correctly converts one deposit and one non-deposit tx without signer or from address",
		depTxNum:    1,
		nonDepTxNum: 1,
	},
	{
		name:        "correctly converts multiple deposit and non-deposit txs without signer or from address",
		depTxNum:    3,
		nonDepTxNum: 3,
	},
	{
		name:        "correctly converts multiple txs with from address but without signer",
		depTxNum:    3,
		nonDepTxNum: 3,
		from:        "from",
	},
	{
		name:        "correctly converts multiple txs with signer but without from address",
		depTxNum:    3,
		nonDepTxNum: 3,
		signTx:      noopSigner,
	},
	{
		name:        "correctly converts multiple txs with both from address and signer",
		depTxNum:    3,
		nonDepTxNum: 3,
		from:        "from",
		signTx:      noopSigner,
	},
}

func noopSigner(_ *tx.Tx) error { return nil }

func TestAdaptPayloadTxsToCosmosTxs(t *testing.T) {
	t.Run("returns empty slice when input txs is nil", func(t *testing.T) {
		txs, err := monomer.AdaptPayloadTxsToCosmosTxs(nil, nil, "")
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("returns empty slice when input txs is empty", func(t *testing.T) {
		txs, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{}, nil, "")
		require.NoError(t, err)
		require.Empty(t, txs)
	})

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

			depositSDKMsgBytes := generateDepositSDKMsgBytes(t, msgAny, test.signTx)
			cosmosTxs := make(bfttypes.Txs, 0, 1+test.nonDepTxNum)
			cosmosTxs = append(cosmosTxs, depositSDKMsgBytes)

			for _, cosmosTx := range txBytes[test.depTxNum:] {
				var txn ethtypes.Transaction
				err := txn.UnmarshalBinary(cosmosTx)
				require.NoError(t, err)
				cosmosTxs = append(cosmosTxs, txn.Data())
			}

			require.Equal(t, cosmosTxs, txs)
		})
	}

	_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	depositTxBytes := testutils.TxToBytes(t, depositTx)
	cosmosEthTxBytes := testutils.TxToBytes(t, cosmosEthTx)
	invalidData := hexutil.Bytes("invalid")

	t.Run("returns error when input tx contains invalid binary data", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{invalidData}, nil, "")
		require.Error(t, err)
	})

	t.Run("returns error when no deposit txs are present", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{cosmosEthTxBytes}, nil, "")
		require.Error(t, err)
	})

	t.Run("returns error when signing tx fails", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{hexutil.Bytes(depositTxBytes)}, func(_ *tx.Tx) error {
			return errors.New("sign tx error")
		}, "")
		require.Error(t, err)
	})

	t.Run("returns error when unable to unmarshal binary tx data", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{hexutil.Bytes(depositTxBytes), hexutil.Bytes(cosmosEthTxBytes), invalidData}, nil, "")
		require.Error(t, err)
	})
}

func TestAdaptCosmosTxsToEthTxs(t *testing.T) {
	t.Run("returns empty slice when input txs is nil", func(t *testing.T) {
		txs, err := monomer.AdaptCosmosTxsToEthTxs(nil)
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("returns empty slice when input txs is empty", func(t *testing.T) {
		txs, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{})
		require.NoError(t, err)
		require.Empty(t, txs)
	})

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

	t.Run("returns error when input contains invalid binary data", func(t *testing.T) {
		_, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{[]byte("invalid")})
		require.Error(t, err)
	})

	t.Run("returns error when unexpected number of messages in Cosmos tx", func(t *testing.T) {
		depositSDKMsgBytes := generateDepositSDKMsgBytes(t, &codectypes.Any{}, nil)
		_, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("returns error when L1 Attributes tx is not found", func(t *testing.T) {
		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{})
		require.NoError(t, err)
		depositSDKMsgBytes := generateDepositSDKMsgBytes(t, msgAny, nil)
		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("returns error when unable to unmarshal MsgApplyL1Txs message", func(t *testing.T) {
		depositSDKMsgBytes := generateDepositSDKMsgBytes(t, &codectypes.Any{Value: []byte("invalid")}, nil)
		_, err := monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("returns error when MsgApplyL1Tx contains only non-deposit txs", func(t *testing.T) {
		txBytes, _, _ := generateTxsAndTxsBytes(t, 0, 5)
		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{TxBytes: txBytes})
		require.NoError(t, err)
		depositSDKMsgBytes := generateDepositSDKMsgBytes(t, msgAny, nil)
		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})

	t.Run("returns error when unable to unmarshal binary data within MsgApplyL1Tx", func(t *testing.T) {
		msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{TxBytes: [][]byte{[]byte("invalid")}})
		require.NoError(t, err)
		depositSDKMsgBytes := generateDepositSDKMsgBytes(t, msgAny, nil)
		_, err = monomer.AdaptCosmosTxsToEthTxs(bfttypes.Txs{depositSDKMsgBytes})
		require.Error(t, err)
	})
}

func generateDepositSDKMsgBytes(t *testing.T, msg *codectypes.Any, signTx monomer.TxSigner) []byte {
	sdkTx := &tx.Tx{Body: &tx.TxBody{Messages: []*codectypes.Any{msg}}}
	if signTx != nil {
		err := signTx(sdkTx)
		require.NoError(t, err)
	}
	depositSDKMsgBytes, err := sdkTx.Marshal()
	require.NoError(t, err)
	return depositSDKMsgBytes
}

func repeat[E any](x E, count int) []E {
	newslice := make([]E, 0, count)
	for range count {
		newslice = append(newslice, x)
	}
	return newslice
}

func generateTxsAndTxsBytes(
	t *testing.T,
	depTxNum int,
	nonDepTxNum int,
) ([][]byte, ethtypes.Transactions, []hexutil.Bytes) {
	_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	depositTxBytes := testutils.TxToBytes(t, depositTx)
	cosmosEthTxBytes := testutils.TxToBytes(t, cosmosEthTx)

	return append(repeat(depositTxBytes, depTxNum), repeat(cosmosEthTxBytes, nonDepTxNum)...),
		append(repeat(depositTx, depTxNum), repeat(cosmosEthTx, nonDepTxNum)...),
		append(repeat(hexutil.Bytes(depositTxBytes), depTxNum), repeat(hexutil.Bytes(cosmosEthTxBytes), nonDepTxNum)...)
}
