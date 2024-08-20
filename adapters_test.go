package monomer_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
)

func TestAdaptPayloadTxsToCosmosTxs(t *testing.T) {
	tests := []struct {
		name          string
		inclusionList map[string]string
		sighTx        monomer.TxSigner
		from          string
	}{
		{
			name: "no txs",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inclusionListTxs := make([]hexutil.Bytes, 0, len(test.inclusionList))
			for _, tx := range testapp.ToTxs(t, test.inclusionList) {
				inclusionListTxs = append(inclusionListTxs, hexutil.Bytes(tx))
			}
			txs, err := monomer.AdaptPayloadTxsToCosmosTxs(inclusionListTxs, test.sighTx, test.from)
			require.NoError(t, err)
			require.Len(t, txs, len(test.inclusionList))
		})
	}
}
