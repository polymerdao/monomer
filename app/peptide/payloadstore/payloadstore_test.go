package payloadstore

import (
	"fmt"
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/eth"
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
	return eetypes.NewPayload(&attrs, hash, height)
}

func TestRollback(t *testing.T) {
	ps := NewPayloadStore()
	ids := make([]*eetypes.PayloadID, 10)

	for h := int64(0); h < 10; h++ {
		payload := dummyPayload(h)
		require.NoError(t, ps.Add(payload))
		id, err := payload.GetPayloadID()
		require.NoError(t, err)
		ids[h] = id
	}

	for h := int64(0); h < 10; h++ {
		newpayload, ok := ps.Get(*ids[h])
		require.True(t, ok)

		id, err := newpayload.GetPayloadID()
		require.NoError(t, err)

		require.Equal(t, ids[h], id)
	}

	require.NoError(t, ps.RollbackToHeight(5))

	for h := int64(0); h < 10; h++ {
		// nothing remains after rollback
		p, ok := ps.Get(*ids[h])
		require.False(t, ok)
		require.Nil(t, p)
	}
}
