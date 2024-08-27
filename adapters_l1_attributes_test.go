package monomer_test

import (
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
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
