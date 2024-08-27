package monomer_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/chaincfg"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestIsL1AttributesTx(t *testing.T) {
	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	invalidToAddress := common.HexToAddress("0x01")

	l1Block := types.NewBlock(&types.Header{
		BaseFee:    big.NewInt(10),
		Difficulty: common.Big0,
		Number:     big.NewInt(0),
		Time:       uint64(0),
	}, nil, nil, nil, trie.NewStackTrie(nil))
	l1InfoRawTx, err := derive.L1InfoDeposit(chaincfg.Mainnet, eth.SystemConfig{}, 0, eth.BlockToInfo(l1Block), l1Block.Time())
	require.NoError(t, err)
	preEcotoneL1InfoTx := types.NewTx(l1InfoRawTx)
	preEcotoneL1InfoTx.SetTime(time.Unix(0, 0))

	tests := []struct {
		name string
		tx   *types.Transaction
		want bool
	}{
		{
			name: "generated l1InfoTx",
			tx:   l1InfoTx,
			want: true,
		},
		{
			name: "generated depositTx",
			tx:   depositTx,
			want: false,
		},
		{
			name: "generated cosmosEthTx",
			tx:   cosmosEthTx,
			want: false,
		},
		{
			name: "to is invalid",
			tx: types.NewTx(&types.DepositTx{
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
			tx: types.NewTx(&types.DepositTx{
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
			tx: types.NewTx(&types.DepositTx{
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
			tx: types.NewTx(&types.DepositTx{
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
			tx: types.NewTx(&types.DepositTx{
				To:                  l1InfoTx.To(),
				Mint:                l1InfoTx.Mint(),
				Value:               l1InfoTx.Value(),
				Gas:                 l1InfoTx.Gas(),
				IsSystemTransaction: true,
				Data:                l1InfoTx.Data(),
			}),
			want: false,
		},
		{
			name: "Before Ecotone update",
			tx:   preEcotoneL1InfoTx,
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, monomer.IsL1AttributesTx(test.tx))
		})
	}
}
