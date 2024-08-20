package monomer_test

import (
	"testing"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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

	t.Run("0 txd", func(t *testing.T) {
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
		err                   error
		signTx                monomer.TxSigner
	}{
		{
			name:     "1 dep tx",
			depTxNum: 1,
		},
		{
			name:        "1 + 1 txs",
			depTxNum:    1,
			nonDepTxNum: 1,
		},
		{
			name:        "10 + 10 txs",
			depTxNum:    10,
			nonDepTxNum: 10,
		},
		{
			name:        "3 + 3 txs + from",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
		},
		{
			name:        "3 + 3 txs + sighTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			signTx:      simpleSigner,
		},
		{
			name:        "3 + 3 txs + from + sighTx",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
			signTx:      simpleSigner,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inclusionListTxs := make([][]byte, 0, test.depTxNum+test.nonDepTxNum)
			_, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)

			depositTxBytes, err := depositTx.MarshalBinary()
			require.NoError(t, err)
			for range test.depTxNum {
				inclusionListTxs = append(inclusionListTxs, depositTxBytes)
			}

			cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
			require.NoError(t, err)
			for range test.nonDepTxNum {
				inclusionListTxs = append(inclusionListTxs, cosmosEthTxBytes)
			}

			txBytes := make([]hexutil.Bytes, 0, test.depTxNum+test.nonDepTxNum)
			for _, tx := range inclusionListTxs {
				txBytes = append(txBytes, hexutil.Bytes(tx))
			}
			txs, err := monomer.AdaptPayloadTxsToCosmosTxs(txBytes, nil, test.from)
			require.NoError(t, err)

			msgAny, err := codectypes.NewAnyWithValue(&rolluptypes.MsgApplyL1Txs{
				TxBytes:     inclusionListTxs[:test.depTxNum],
				FromAddress: test.from,
			})
			require.NoError(t, err)

			sdkTx := sdktx.Tx{
				Body: &sdktx.TxBody{
					Messages: []*codectypes.Any{msgAny},
				},
			}

			depositSDKMsgBytes, err := sdkTx.Marshal()
			require.NoError(t, err)

			cosmosTxs := make(bfttypes.Txs, 0, 1+test.nonDepTxNum)
			cosmosTxs = append(cosmosTxs, depositSDKMsgBytes)

			for _, cosmosTx := range inclusionListTxs[test.depTxNum:] {
				var tx ethtypes.Transaction
				err := tx.UnmarshalBinary(cosmosTx)
				require.NoError(t, err)
				cosmosTxs = append(cosmosTxs, tx.Data())
			}

			require.Equal(t, cosmosTxs, txs)
		})
	}
}
