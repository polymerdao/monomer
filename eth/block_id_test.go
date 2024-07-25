package eth_test

import (
	"encoding/json"
	"fmt"
	"testing"

	bfttypes "github.com/cometbft/cometbft/types"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/monomerdb"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestBlockIDUnmarshalValidJSON(t *testing.T) {
	tests := []struct {
		json string
		want eth.BlockID
	}{
		{
			json: "latest",
			want: eth.BlockID{
				Label: opeth.Unsafe,
			},
		},
		{
			json: "safe",
			want: eth.BlockID{
				Label: opeth.Safe,
			},
		},
		{
			json: "finalized",
			want: eth.BlockID{
				Label: opeth.Finalized,
			},
		},
		{
			json: "0x0",
			want: eth.BlockID{},
		},
		{
			json: "0x1",
			want: eth.BlockID{
				Height: 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.json, func(t *testing.T) {
			got := eth.BlockID{}
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf("%q", test.json)), &got))
			require.Equal(t, test.want, got)
		})
	}
}

func TestBlockIDGet(t *testing.T) {
	blockStore := testutils.NewLocalMemDB(t)
	block, err := monomer.MakeBlock(&monomer.Header{}, bfttypes.Txs{})
	require.NoError(t, err)
	require.NoError(t, blockStore.AppendBlock(block))
	require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))

	tests := map[string]struct {
		id   eth.BlockID
		want *monomer.Block
	}{
		"unsafe exists": {
			id: eth.BlockID{
				Label: opeth.Unsafe,
			},
			want: block,
		},
		"safe exists": {
			id: eth.BlockID{
				Label: opeth.Safe,
			},
			want: block,
		},
		"finalized exists": {
			id: eth.BlockID{
				Label: opeth.Finalized,
			},
			want: block,
		},
		"height 0 exists": {
			id: eth.BlockID{
				Height: 0,
			},
			want: block,
		},
		"height 1 does not exist": {
			id: eth.BlockID{
				Height: 1,
			},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			got, err := test.id.Get(blockStore)
			if test.want == nil {
				require.Nil(t, got)
				require.ErrorIs(t, err, monomerdb.ErrNotFound)
				return
			}
			require.NoError(t, err)
			require.Equal(t, got, test.want)
		})
	}
}
