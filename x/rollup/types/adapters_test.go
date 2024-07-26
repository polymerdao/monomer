package types_test

import (
	"math/big"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdaptPayloadTxsToCosmosTxs is a test function that demonstrates how to use
// the AdaptPayloadTxsToCosmosTxs function from the rolluptypes package.
//
// The function creates a deposit transaction, marshals it into a binary format,
// converts it to a Cosmos transaction, and then unmarshals it back into a
// deposit transaction. The resulting deposit transaction is then compared to the
// original one to ensure that the conversion process was successful.
func TestAdaptPayloadTxsToCosmosTxs(t *testing.T) {
	// TODO: create an inner generator function to generate the test data
	// Define the necessary transaction parameters.
	sourceHashString := "0x1234567890abcdef1234567890abcdef12345678"
	fromAddressString := "0x1111111111111111111111111111111111111111"
	toAddressString := "cosmos1recipientaddress"
	value64 := int64(1_000_000_000_000_000_000) // 1 Ether = 1000000 uatom
	gas := uint64(1_000_000)
	data := []byte("test data")
	mint := big.NewInt(1_000_000_000_000_000_000) // 1 Ether = 1000000 uatom
	isSystemTransaction := false

	// Convert the parameter values to Ethereum types.
	sourceHash := common.HexToHash(sourceHashString)
	fromAddress := common.HexToAddress(fromAddressString)
	toAddress := common.HexToAddress(toAddressString)
	value := big.NewInt(value64)

	// Create a deposit transaction.
	depInner := &types.DepositTx{
		SourceHash:          sourceHash,
		From:                fromAddress,
		To:                  &toAddress,
		Value:               value,
		Gas:                 gas,
		Data:                data,
		Mint:                mint,
		IsSystemTransaction: isSystemTransaction,
	}

	testTable := []struct {
		name   string
		inners []types.TxData
	}{
		{
			name:   "Deposit Transaction",
			inners: []types.TxData{depInner},
		},
	}

	for _, tc := range testTable {
		t.Run(tc.name, func(t *testing.T) {
			ethTxs := make([]hexutil.Bytes, len(tc.inners))
			transactions := make([]*types.Transaction, len(tc.inners))

			for i, inner := range tc.inners {
				transactions[i] = types.NewTx(inner)
				txBinary, err := transactions[i].MarshalBinary()
				require.NoError(t, err)
				ethTxs[i] = txBinary
			}

			// Convert the binary format to a Cosmos transaction.
			cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs(ethTxs)
			require.NoError(t, err)

			// Unmarshal the first Cosmos transaction back into a deposit transaction.
			for i, cosmosTx := range cosmosTxs {
				decodedTx, err := unmarshalTx(cosmosTx)
				require.NoError(t, err)

				protoCodec := makeProtoCodec()
				var applyL1TxsRequest rollupv1.ApplyL1TxsRequest
				err = protoCodec.Unmarshal(decodedTx.Body.Messages[0].Value, &applyL1TxsRequest)
				require.NoError(t, err)

				newTransaction := transactions[i] // Copy the original transaction because time fields are different if not copied.
				err = newTransaction.UnmarshalBinary(applyL1TxsRequest.TxBytes[0])
				require.NoError(t, err)

				assert.Equal(t, transactions[i], newTransaction)
			}
		})
	}
}

func registerInterfaces(interfaceRegistry codectypes.InterfaceRegistry) {
	rollupv1.RegisterInterfaces(interfaceRegistry)
}

func makeProtoCodec() codec.ProtoCodecMarshaler {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	registerInterfaces(interfaceRegistry)
	cdc := codec.NewProtoCodec(interfaceRegistry)
	return cdc
}

func unmarshalTx(txBytes []byte) (*tx.Tx, error) {
	// Initialize the codec
	protoCodec := makeProtoCodec()

	// Create a variable to hold the transaction
	var transaction tx.Tx

	// Unmarshal the transaction bytes
	err := protoCodec.Unmarshal(txBytes, &transaction)
	if err != nil {
		return nil, err
	}

	return &transaction, nil
}
