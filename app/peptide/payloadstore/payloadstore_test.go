package payloadstore

import (
	"fmt"
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/stretchr/testify/require"
)

func dummyPayload(height int64) *monomer.Payload {
	gaslimit := hexutil.Uint64(1)
	attrs := eth.PayloadAttributes{
		GasLimit: &gaslimit,
	}
	hash := common.HexToHash(fmt.Sprintf("0x%x", height))
	return &monomer.Payload{
		Timestamp:             uint64(attrs.Timestamp),
		PrevRandao:            attrs.PrevRandao,
		SuggestedFeeRecipient: attrs.SuggestedFeeRecipient,
		Withdrawals:           attrs.Withdrawals,
		NoTxPool:              attrs.NoTxPool,
		GasLimit:              uint64(*attrs.GasLimit),
		ParentBeaconBlockRoot: attrs.ParentBeaconBlockRoot,
		ParentHash:            hash,
		Height:                height,
		Transactions:          attrs.Transactions,
	}
}

func dummyPayloadStore(t *testing.T, n int64) (*Store, []*engine.PayloadID) {
	ps := New()
	ids := make([]*engine.PayloadID, n)

	// add some payloads
	for h := int64(0); h < n; h++ {
		payload := dummyPayload(h)
		ps.Add(payload)
		ids[h] = payload.ID()
	}

	// assert non-emptiness
	for h := int64(0); h < n; h++ {
		newpayload, ok := ps.Get(*ids[h])
		require.True(t, ok)
		require.Equal(t, ids[h], newpayload.ID())
	}
	require.NotNil(t, ps.Current())

	return ps, ids
}

func TestClear(t *testing.T) {
	ps, ids := dummyPayloadStore(t, 10)
	ps.Clear()

	for h := int64(0); h < 10; h++ {
		// nothing remains after Clear
		p, ok := ps.Get(*ids[h])
		require.False(t, ok)
		require.Nil(t, p)
	}

	// current is nil after Clear
	require.Nil(t, ps.Current())
}

func TestRollback(t *testing.T) {
	// test batch removal of payloads
	ps, ids := dummyPayloadStore(t, 10)
	ps.RollbackToHeight(5)

	for h := int64(0); h < 10; h++ {
		// payloads with height > 5 are removed
		p, ok := ps.Get(*ids[h])
		if h > 5 {
			require.False(t, ok)
			require.Nil(t, p)
		} else {
			require.True(t, ok)
			require.Equal(t, ids[h], p.ID())
		}
	}
	current := ps.Current()
	require.Equal(t, int64(5), current.Height)
	require.Equal(t, ids[5], current.ID())

	// test no existing payload at height
	require.Error(t, ps.RollbackToHeight(15))
}

func TestAdd(t *testing.T) {
	fresh, stale := int64(10), int64(5)

	ps := New()
	freshPayload := dummyPayload(fresh)
	ps.Add(freshPayload)

	// assert inclusion of fresh payload
	retrieved, ok := ps.Get(*freshPayload.ID())
	require.True(t, ok)
	require.Equal(t, freshPayload.ID(), retrieved.ID())
	require.NotNil(t, ps.Current())

	stalePayload := dummyPayload(stale)
	ps.Add(stalePayload)

	// assert inclusion of stale payload
	retrieved, ok = ps.Get(*stalePayload.ID())
	require.True(t, ok)
	require.Equal(t, stalePayload.ID(), retrieved.ID())

	// assert current not updated with stale payload
	require.Equal(t, fresh, ps.Current().Height)
}

func TestRemove(t *testing.T) {
	ps, ids := dummyPayloadStore(t, 10)
	ps.Remove(*ids[5])

	// assert removal of payload at height 5
	p, ok := ps.Get(*ids[5])
	require.False(t, ok)
	require.Nil(t, p)

	// assert current remains the same
	require.Equal(t, int64(9), ps.Current().Height)
	require.Equal(t, ids[9], ps.Current().ID())

	// remove current payload
	ps.Remove(*ids[9])

	// assert current is updated
	require.Equal(t, int64(8), ps.Current().Height)
	require.Equal(t, ids[8], ps.Current().ID())
}
