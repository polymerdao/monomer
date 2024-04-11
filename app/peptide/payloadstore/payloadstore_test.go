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
	ps := NewPayloadStore()
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
