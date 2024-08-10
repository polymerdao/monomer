package monomer_test

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"testing"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	optestutils "github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/kataras/iris/v12/x/errors"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

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

	noopSigner := func(_ *sdktx.Tx) error {
		return nil
	}

	tests := []struct {
		name, from            string
		depTxNum, nonDepTxNum int
		signTx                monomer.TxSigner
	}{
		{
			name:     "converts single deposit tx without signer or from address",
			depTxNum: 1,
		},
		{
			name:        "converts one deposit and one non-deposit tx without signer or from address",
			depTxNum:    1,
			nonDepTxNum: 1,
		},
		{
			name:        "converts multiple deposit and non-deposit txs without signer or from address",
			depTxNum:    10,
			nonDepTxNum: 10,
		},
		{
			name:        "converts multiple txs with from address but without signer",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
		},
		{
			name:        "converts multiple txs with signer but without from address",
			depTxNum:    3,
			nonDepTxNum: 3,
			signTx:      noopSigner,
		},
		{
			name:        "converts multiple txs with both from address and signer",
			depTxNum:    3,
			nonDepTxNum: 3,
			from:        "from",
			signTx:      noopSigner,
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

	t.Run("returns error when input tx contains invalid binary data", func(t *testing.T) {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{hexutil.Bytes("invalid")}, nil, "")
		require.Error(t, err)
	})

	t.Run("returns error when no deposit txs are present", func(t *testing.T) {
		_, _, cosmosEthTx := testutils.GenerateEthTxs(t)
		cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
		require.NoError(t, err)
		_, err = monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{cosmosEthTxBytes}, nil, "")
		require.Error(t, err)
	})

	t.Run("returns error when signing tx fails", func(t *testing.T) {
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

	t.Run("returns error when unable to unmarshal binary tx data", func(t *testing.T) {
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

	noopSigner := func(_ *sdktx.Tx) error {
		return nil
	}

	tests := []struct {
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
			depTxNum:    10,
			nonDepTxNum: 10,
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

	t.Run("returns error when L1 Attributes tx is not found", func(t *testing.T) {
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

	t.Run("returns error when unable to unmarshal MsgL1Txs message", func(t *testing.T) {
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

	t.Run("returns error when MsgL1Tx contains only non-deposit txs", func(t *testing.T) {
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

	t.Run("returns error when unable to unmarshal binary data within MsgL1Tx", func(t *testing.T) {
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

func BenchmarkAdaptPayloadTxsToCosmosTxs(b *testing.B) {
	rng := rand.New(rand.NewSource(1234))
	depositTx := gethtypes.NewTx(optestutils.GenerateDeposit(optestutils.RandomHash(rng), rng))
	cosmosEthTx := monomer.AdaptNonDepositCosmosTxToEthTx([]byte{1})

	depositTxBytes, err := depositTx.MarshalBinary()
	require.NoError(b, err)
	hexTxBytes := make([]hexutil.Bytes, 0, 1000)

	for range 500 {
		hexTxBytes = append(hexTxBytes, hexutil.Bytes(depositTxBytes))
	}

	cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
	require.NoError(b, err)
	for range 500 {
		hexTxBytes = append(hexTxBytes, hexutil.Bytes(cosmosEthTxBytes))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := monomer.AdaptPayloadTxsToCosmosTxs(hexTxBytes, nil, "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAdaptCosmosTxsToEthTxs(b *testing.B) {
	rng := rand.New(rand.NewSource(1234))
	depositTx := gethtypes.NewTx(optestutils.GenerateDeposit(optestutils.RandomHash(rng), rng))
	cosmosEthTx := monomer.AdaptNonDepositCosmosTxToEthTx([]byte{1})

	depositTxBytes, err := depositTx.MarshalBinary()
	require.NoError(b, err)
	hexTxBytes := make([]hexutil.Bytes, 0, 1000)

	for range 500 {
		hexTxBytes = append(hexTxBytes, hexutil.Bytes(depositTxBytes))
	}

	cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
	require.NoError(b, err)
	for range 500 {
		hexTxBytes = append(hexTxBytes, hexutil.Bytes(cosmosEthTxBytes))
	}

	txs, err := monomer.AdaptPayloadTxsToCosmosTxs(hexTxBytes, nil, "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := monomer.AdaptCosmosTxsToEthTxs(txs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestABI(t *testing.T) {
	// The ABI of the L1Block contract (replace with the actual ABI)
	const l1BlockABI = `[
						{
							"inputs": [],
							"name": "setL1BlockValuesEcotone",
							"outputs": [],
							"stateMutability": "nonpayable",
							"type": "function"
						}
					],`

	// Example transaction data (replace with actual data)
	// txData := common.Hex2Bytes(string(tx.Data()))

	// Parse the ABI
	parsedABI, err := abi.JSON(bytes.NewReader([]byte(l1BlockABI)))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	fmt.Printf("%+v\n\n\n", parsedABI)

	// Get the method by name
	method := parsedABI.Methods["setL1BlockValuesEcotone"]

	fmt.Printf("%+v\n", method)
	methodID := method.ID
	fmt.Println(bytes.Equal(methodID, []byte{68, 10, 94, 32}))

	invalidTxData := []byte{0x01, 0x02, 0x03, 0x04}

	// Check if the transaction data starts with the expected method ID
	if len(invalidTxData) < len(methodID) || !bytes.Equal(invalidTxData[:len(methodID)], methodID) {
		fmt.Println("Data is NOT a call to the setL1BlockValues function.")
	}
}
