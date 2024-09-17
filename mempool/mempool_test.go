package mempool_test

import (
	"testing"

	comettypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMempool(t *testing.T) {
	pool := mempool.New(testutils.NewMemDB(t))

	t.Run("empty pool", func(t *testing.T) {
		l, err := pool.Len()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), l)

		_, err = pool.Dequeue()
		require.Error(t, err)
	})

	t.Run("deposit transaction", func(t *testing.T) {
		_, depositTx, _ := testutils.GenerateEthTxs(t)
		depositTxBytes, err := depositTx.MarshalBinary()
		require.NoError(t, err)

		cosmosTxs, err := monomer.AdaptPayloadTxsToCosmosTxs([]hexutil.Bytes{depositTxBytes}, nil, "")
		require.NoError(t, err)
		require.ErrorContains(t, pool.Enqueue(cosmosTxs[0]), "deposit txs are not allowed in the pool")
	})

	// enqueue multiple to empty
	for i := byte(0); i < 3; i++ {
		require.NoError(t, pool.Enqueue(comettypes.Tx([]byte{i})))

		l, err := pool.Len()
		require.NoError(t, err)
		assert.Equal(t, uint64(i)+1, l)
	}

	// consume some
	for i := byte(0); i < 2; i++ {
		txn, err := pool.Dequeue()
		require.NoError(t, err)
		require.Equal(t, comettypes.Tx([]byte{i}), txn)

		l, err := pool.Len()
		require.NoError(t, err)
		require.Equal(t, 3-uint64(i)-1, l)
	}

	// push multiple to non empty
	for i := byte(3); i < 5; i++ {
		require.NoError(t, pool.Enqueue(comettypes.Tx([]byte{i})))

		l, err := pool.Len()
		require.NoError(t, err)
		require.Equal(t, uint64(i)-1, l)
	}

	// consume all
	for i := byte(2); i < 5; i++ {
		txn, err := pool.Dequeue()
		require.NoError(t, err)
		require.Equal(t, comettypes.Tx([]byte{i}), txn)
	}

	l, err := pool.Len()
	require.NoError(t, err)
	require.Equal(t, uint64(0), l)

	_, err = pool.Dequeue()
	require.Error(t, err)
}
