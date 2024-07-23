package monomer_test

import (
	"testing"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestBlockToEth(t *testing.T) {
	l1InfoTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(t)
	block := testutils.GenerateBlockFromEthTxs(t, l1InfoTx, []*ethtypes.Transaction{depositTx}, []*ethtypes.Transaction{cosmosEthTx})
	ethBlock, err := block.ToEth()
	require.NoError(t, err)
	for i, ethTx := range []*ethtypes.Transaction{l1InfoTx, depositTx, cosmosEthTx} {
		require.EqualExportedValues(t, ethTx, ethBlock.Body().Transactions[i])
	}
}
