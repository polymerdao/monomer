package eth_test

import (
	"encoding/json"
	"fmt"
	"testing"

	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/eth"
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
	blockStore := store.NewBlockStore(testutils.NewMemDB(t))
	block, err := monomer.MakeBlock(&monomer.Header{}, nil)
	require.NoError(t, err)
	blockStore.AddBlock(block)
	require.NoError(t, blockStore.UpdateLabel(opeth.Unsafe, block.Header.Hash))

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
		"safe does not exist": {
			id: eth.BlockID{
				Label: opeth.Safe,
			},
		},
		"finalized does not exist": {
			id: eth.BlockID{
				Label: opeth.Finalized,
			},
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
			require.Equal(t, test.id.Get(blockStore), test.want)
		})
	}
}
