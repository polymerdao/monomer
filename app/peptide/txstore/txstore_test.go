package txstore

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	dbm "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

func dummyTxs(height int64, count int) []*abcitypes.TxResult {
	result := make([]*abcitypes.TxResult, count)
	for i := 0; i < count; i++ {
		result[i] = &abcitypes.TxResult{
			Height: height,
			Tx:     []byte(fmt.Sprintf("h:%v|i:%v", height, i)),
			Result: abcitypes.ResponseDeliverTx{},
		}
	}
	return result
}

func TestRollback(t *testing.T) {
	for _, tc := range []struct {
		desc  string
		start int64
		end   int64
	}{
		{
			desc:  "from zero to fifteen",
			start: 0,
			end:   15,
		},
		{
			desc:  "across 100",
			start: 90,
			end:   120,
		},
		{
			desc:  "more than 300 keys",
			start: 1000600,
			end:   1000900,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			txs := NewTxStore(dbm.NewMemDB())
			hashes := make(map[int64][][]byte)
			r := rand.New(rand.NewSource(time.Now().UnixNano()))

			require.Less(t, tc.start, tc.end)
			for h := tc.start; h < tc.end; h++ {
				txsPerBlock := r.Intn(10)
				res := dummyTxs(h, txsPerBlock)
				for i := 0; i < txsPerBlock; i++ {
					tx := bfttypes.Tx(res[i].GetTx())
					hashes[h] = append(hashes[h], tx.Hash())
				}
				err := txs.Add(res)
				require.NoError(t, err)
			}

			for h := tc.start; h < tc.end; h++ {
				for _, hash := range hashes[h] {
					res, err := txs.Get(hash)
					require.NotNil(t, res)
					require.NoError(t, err)
				}
			}

			rollbackHeight := tc.start + int64((tc.end-tc.start)/2)
			require.NoError(t, txs.RollbackToHeight(rollbackHeight, tc.end))

			for h := tc.start; h < tc.end; h++ {
				for _, hash := range hashes[h] {
					res, err := txs.Get(hash)
					require.NoError(t, err)
					if h <= rollbackHeight {
						require.NotNil(t, res, fmt.Sprintf("height: %d, rollback: %d", h, rollbackHeight))
					} else {
						require.Nil(t, res, fmt.Sprintf("height: %d, rollback: %d", h, rollbackHeight))
					}
				}
			}
		})
	}
}
