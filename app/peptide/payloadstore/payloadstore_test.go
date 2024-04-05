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

func TestRollback(t *testing.T) {
	ps := NewPayloadStore()
	ids := make([]*engine.PayloadID, 10)

	for h := int64(0); h < 10; h++ {
		payload := dummyPayload(h)
		ps.Add(payload)
		ids[h] = payload.ID()
	}

	for h := int64(0); h < 10; h++ {
		newpayload, ok := ps.Get(*ids[h])
		require.True(t, ok)
		require.Equal(t, ids[h], newpayload.ID())
	}

	require.NoError(t, ps.RollbackToHeight(5))

	for h := int64(0); h < 10; h++ {
		// nothing remains after rollback
		p, ok := ps.Get(*ids[h])
		require.False(t, ok)
		require.Nil(t, p)
	}
}
