package types_test

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptPayloadTxsToCosmosTxs(t *testing.T) {
	t.Run("Zero txs", func(t *testing.T) {
		cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(cosmosTxs))
	})

	src := rand.NewSource(0)
	r := rand.New(src)

	t.Run("non-zero txs without error", func(t *testing.T) {
		testTable := []struct {
			name   string
			inners []ethtypes.TxData
		}{
			{
				name:   "DepositTx",
				inners: generateMultipleDepositTx(r, 1),
			},
			{
				name:   "Multiple DepositTxs",
				inners: generateMultipleDepositTx(r, 3),
			},
			{
				name:   "DepositTx + AccessListTx",
				inners: []ethtypes.TxData{generateDepositTx(r), generateDynamicFeeTx(r)},
			},
			{
				name:   "Multiple DepositTxs + DynamicFeeTxs",
				inners: append(generateMultipleDepositTx(r, 3), generateMultipleDynamicFeeTx(r, 3)...),
			},
		}

		interfaceRegistry := codectypes.NewInterfaceRegistry()
		rollupv1.RegisterInterfaces(interfaceRegistry)
		protoCodec := codec.NewProtoCodec(interfaceRegistry)

		for _, tc := range testTable {
			t.Run(tc.name, func(t *testing.T) {
				ethTxs := make([]hexutil.Bytes, len(tc.inners))
				transactions := make([]*ethtypes.Transaction, len(tc.inners))

				depositTxsNum := 0
				for i, inner := range tc.inners {
					transactions[i] = ethtypes.NewTx(inner)
					txBinary, err := transactions[i].MarshalBinary()
					require.NoError(t, err)
					ethTxs[i] = txBinary
					if transactions[i].IsDepositTx() {
						depositTxsNum++
					}
				}

				// Convert the binary format to a Cosmos transaction.
				cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs(ethTxs)
				require.NoError(t, err)

				if len(ethTxs) == 0 {
					assert.Equal(t, 0, len(cosmosTxs))
					return
				}

				var decodedTx sdktx.Tx
				err = decodedTx.Unmarshal(cosmosTxs[0])
				require.NoError(t, err)

				var applyL1TxsRequest rollupv1.ApplyL1TxsRequest
				err = protoCodec.Unmarshal(decodedTx.GetBody().GetMessages()[0].GetValue(), &applyL1TxsRequest)
				require.NoError(t, err)

				// Copy the original transaction because time fields are different if not copied.
				assert.Equal(t, depositTxsNum, len(applyL1TxsRequest.TxBytes))
				for i, txBytes := range applyL1TxsRequest.TxBytes {
					newTransaction := transactions[i]
					err = newTransaction.UnmarshalBinary(txBytes)
					require.NoError(t, err)
					assert.Equal(t, transactions[i], newTransaction)
				}

				for i := 1; i < len(cosmosTxs); i++ {
					assert.Equal(t, transactions[depositTxsNum-1+i].Data(), []byte(cosmosTxs[i]))
				}
			})
		}
	})

	t.Run("non-zero txs with error", func(t *testing.T) {
		t.Run("unmarshal binary error", func(t *testing.T) {
			cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{[]byte("invalid")})
			assert.Nil(t, cosmosTxs)
			assert.ErrorContains(t, err, "unmarshal binary")
		})
		t.Run("zero deposit txs", func(t *testing.T) {
			inner := generateDynamicFeeTx(r)
			transaction := ethtypes.NewTx(inner)
			txBytes, err := transaction.MarshalBinary()
			require.NoError(t, err)
			cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{txBytes})
			assert.Nil(t, cosmosTxs)
			assert.Error(t, err)
		})
		t.Run("NewAnyWithValue error", func(t *testing.T) {
			t.Skip()
			// TODO: Implement this test case
		})
		t.Run("depositSDKMsgBytes marshal error", func(t *testing.T) {
			t.Skip()
			// TODO: Implement this test case
		})
		t.Run("Unpack Cosmos txs error", func(t *testing.T) {
			depInner := generateDepositTx(r)
			depTx := ethtypes.NewTx(depInner)
			depTxBytes, err := depTx.MarshalBinary()
			require.NoError(t, err)

			nonDepInner := generateDynamicFeeTx(r)
			nonDepTx := ethtypes.NewTx(nonDepInner)
			nonDepTxBytes, err := nonDepTx.MarshalBinary()
			require.NoError(t, err)

			cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{depTxBytes, nonDepTxBytes, []byte("invalid")})
			assert.Nil(t, cosmosTxs)
			assert.ErrorContains(t, err, "unmarshal binary tx: ")
		})
	})
}

func generateMultipleDepositTx(r *rand.Rand, n int) []ethtypes.TxData {
	transactions := make([]ethtypes.TxData, n)
	for i := 0; i < n; i++ {
		transactions[i] = generateDepositTx(r)
	}
	return transactions
}

func generateDepositTx(r *rand.Rand) ethtypes.TxData {
	toAddress := generateAddress(r)
	return &ethtypes.DepositTx{
		SourceHash:          generateHash(r),
		From:                generateAddress(r),
		To:                  &toAddress,
		Value:               generateBigInt(r),
		Gas:                 r.Uint64(),
		Data:                generateData(r),
		Mint:                generateBigInt(r),
		IsSystemTransaction: false,
	}
}

func generateMultipleDynamicFeeTx(r *rand.Rand, n int) []ethtypes.TxData {
	transactions := make([]ethtypes.TxData, n)
	for i := 0; i < n; i++ {
		transactions[i] = generateDynamicFeeTx(r)
	}
	return transactions
}

func generateDynamicFeeTx(r *rand.Rand) ethtypes.TxData {
	toAddress := generateAddress(r)
	return &ethtypes.DynamicFeeTx{
		ChainID:    generateBigInt(r),
		Nonce:      r.Uint64(),
		GasTipCap:  generateBigInt(r),
		GasFeeCap:  generateBigInt(r),
		Gas:        r.Uint64(),
		To:         &toAddress,
		Value:      generateBigInt(r),
		Data:       generateData(r),
		AccessList: nil,
		V:          generateBigInt(r),
		R:          generateBigInt(r),
		S:          generateBigInt(r),
	}
}

func generateHash(r *rand.Rand) common.Hash {
	return common.BigToHash(big.NewInt(r.Int63()))
}

func generateAddress(r *rand.Rand) common.Address {
	return common.BigToAddress(big.NewInt(r.Int63()))
}

func generateBigInt(r *rand.Rand) *big.Int {
	return big.NewInt(r.Int63())
}

func generateData(r *rand.Rand) []byte {
	data := make([]byte, r.Intn(100))
	for i := range data {
		data[i] = byte(r.Intn(256))
	}
	return data
}
