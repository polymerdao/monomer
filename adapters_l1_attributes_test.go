package monomer_test

import (
	"bytes"
	"fmt"
	"log"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestIsL1AttributesTx(t *testing.T) {
	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	invalidToAddress := common.HexToAddress("0x01")

	tests := []struct {
		name string
		tx   types.Transaction
		want bool
	}{
		{
			name: "generated l1InfoTx",
			tx:   *l1InfoTx,
			want: true,
		},
		{
			name: "generated depositTx",
			tx:   *depositTx,
			want: false,
		},
		{
			name: "generated cosmosEthTx",
			tx:   *cosmosEthTx,
			want: false,
		},
		{
			name: "to is invalid",
			tx: *types.NewTx(&types.DepositTx{
				To:                  &invalidToAddress,
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 l1InfoTx.Gas(),
				IsSystemTransaction: l1InfoTx.IsSystemTx(),
				Data:                l1InfoTx.Data(),
			}),
			want: false,
		},
		{
			name: "gas is invalid",
			tx: *types.NewTx(&types.DepositTx{
				To:                  l1InfoTx.To(),
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 150_000_000,
				IsSystemTransaction: l1InfoTx.IsSystemTx(),
				Data:                l1InfoTx.Data(),
			}),
			want: false,
		},
		{
			name: "data is short",
			tx: *types.NewTx(&types.DepositTx{
				To:                  l1InfoTx.To(),
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 l1InfoTx.Gas(),
				IsSystemTransaction: l1InfoTx.IsSystemTx(),
				Data:                l1InfoTx.Data()[:len(derive.L1InfoFuncEcotoneSignature)],
			}),
			want: false,
		},
		{
			name: "data is invalid",
			tx: *types.NewTx(&types.DepositTx{
				To:                  l1InfoTx.To(),
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 l1InfoTx.Gas(),
				IsSystemTransaction: l1InfoTx.IsSystemTx(),
				Data:                []byte("invalid"),
			}),
			want: false,
		},
		{
			name: "IsSystemTransaction is true",
			tx: *types.NewTx(&types.DepositTx{
				To:                  l1InfoTx.To(),
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 l1InfoTx.Gas(),
				IsSystemTransaction: true,
				Data:                l1InfoTx.Data(),
			}),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, monomer.IsL1AttributesTx(&test.tx))
		})
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
