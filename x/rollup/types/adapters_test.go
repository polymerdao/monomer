package types_test

import (
	"fmt"
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

	// Convert the deposit transaction to a binary format.
	depTx := types.NewTx(depInner)

	depTxBinary, err := depTx.MarshalBinary()
	require.NoError(t, err)

	// Convert the binary format to a Cosmos transaction.
	cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{depTxBinary})
	require.NoError(t, err)

	// Unmarshal the first Cosmos transaction back into a deposit transaction.
	decodedTx, err := unmarshalTx(cosmosTxs[0])
	if err != nil {
		require.NoError(t, err)
	}
	// Print the decoded transaction for debugging purposes.
	fmt.Printf("%+v\n", decodedTx)

	// Print the value of the first message in the decoded transaction for debugging purposes.
	// body:<messages:<type_url:"/rollup.v1.ApplyL1TxsRequest" value:"\no~\370l\240\000\000\000\000\000\000\000\000\000\000\000\000\0224Vx\220\253\315\357\0224Vx\220\253\315\357\0224Vx\224\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\021\224\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\014\210\r\340\266\263\247d\000\000\210\r\340\266\263\247d\000\000\203\017B@\200\211test data" > >
	fmt.Println()
	fmt.Printf("%x\n", decodedTx.Body.Messages[0].Value)
	// 0a6f7ef86ca00000000000000000000000001234567890abcdef1234567890abcdef1234567894111111111111111111111111111111111111111194000000000000000000000000000000000000000c880de0b6b3a7640000880de0b6b3a7640000830f42408089746573742064617461
	fmt.Println()

	// Unmarshal the first message in the decoded transaction back into a deposit transaction.
	decodedTx, err = unmarshalTx(decodedTx.Body.Messages[0].Value)
	require.NoError(t, err)

	//  Error Trace:    /Users/daniilankusin/monomer/x/rollup/types/adapters_test.go:64
	//             Error:          Received unexpected error:
	//                             proto: illegal wireType 6

	fmt.Printf("%+v\n", decodedTx)
	fmt.Println()

	// Unmarshal the first message in the decoded transaction back into a deposit transaction.
	err = depTx.UnmarshalBinary(decodedTx.Body.Messages[0].Value)
	require.NoError(t, err)

	// Error Trace:    /Users/daniilankusin/monomer/x/rollup/types/adapters_test.go:75
	//             Error:          Received unexpected error:
	//                             transaction type not supported
	//             Test:           TestAdaptPayloadTxsToCosmosTxs

	fmt.Printf("%+v\n", depTx)
	fmt.Println()
}

func registerInterfaces(interfaceRegistry codectypes.InterfaceRegistry) {
	// Register SDK interfaces and concrete types
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
