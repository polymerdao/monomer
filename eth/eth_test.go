package eth_test

import (
	"context"
	"math/big"
	"testing"

	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestChainId(t *testing.T) {
	for _, id := range []monomer.ChainID{0, 1, 2, 10} {
		t.Run(id.String(), func(t *testing.T) {
			hexID := id.HexBig()
			require.Equal(t, hexID, eth.NewChainID(hexID, eth.NewNoopMetrics()).ChainId())
		})
	}
}

func TestGetBlockByNumber(t *testing.T) {
	blockStore := testutils.NewLocalMemDB(t)

	block := testutils.GenerateBlock(t)
	wantEthBlock, err := block.ToEth()
	require.NoError(t, err)

	require.NoError(t, blockStore.AppendBlock(block))
	require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))

	tests := map[string]struct {
		id   eth.BlockID
		want *ethtypes.Block
	}{
		"unsafe block exists": {
			id: eth.BlockID{
				Label: opeth.Unsafe,
			},
			want: wantEthBlock,
		},
		"safe block exists": {
			id: eth.BlockID{
				Label: opeth.Safe,
			},
			want: wantEthBlock,
		},
		"finalized block exists": {
			id: eth.BlockID{
				Label: opeth.Finalized,
			},
			want: wantEthBlock,
		},
		"block 0 exists": {
			id:   eth.BlockID{},
			want: wantEthBlock,
		},
		"block 1 does not exist": {
			id: eth.BlockID{
				Height: 1,
			},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			for description, fullTxs := range map[string]bool{
				"include txs": true,
				"exclude txs": false,
			} {
				t.Run(description, func(t *testing.T) {
					chainID := new(big.Int)
					s := eth.NewBlock(blockStore, chainID, eth.NewNoopMetrics())
					got, err := s.GetBlockByNumber(test.id, fullTxs)
					if test.want == nil {
						require.ErrorIs(t, err, ethereum.NotFound)
						require.Nil(t, got)
						return
					}
					require.NoError(t, err)
					want, err := ethapi.SimpleRPCMarshalBlock(test.want, fullTxs, chainID)
					require.NoError(t, err)
					require.Equal(t, want, got)
				})
			}
		})
	}
}

func TestGetBlockByHash(t *testing.T) {
	blockStore := testutils.NewLocalMemDB(t)
	block := testutils.GenerateBlock(t)
	require.NoError(t, blockStore.AppendBlock(block))

	for description, fullTx := range map[string]bool{
		"include txs": true,
		"exclude txs": false,
	} {
		t.Run(description, func(t *testing.T) {
			t.Run("block hash 1 exists", func(t *testing.T) {
				chainID := new(big.Int)
				e := eth.NewBlock(blockStore, chainID, eth.NewNoopMetrics())
				got, err := e.GetBlockByHash(block.Header.Hash, fullTx)
				require.NoError(t, err)
				ethBlock, err := block.ToEth()
				require.NoError(t, err)
				want, err := ethapi.SimpleRPCMarshalBlock(ethBlock, fullTx, chainID)
				require.NoError(t, err)
				require.Equal(t, want, got)
			})
		})
		t.Run("block hash 0 does not exist", func(t *testing.T) {
			for description, inclTx := range map[string]bool{
				"include txs": true,
				"exclude txs": false,
			} {
				t.Run(description, func(t *testing.T) {
					chainID := new(big.Int)
					e := eth.NewBlock(blockStore, chainID, eth.NewNoopMetrics())
					got, err := e.GetBlockByHash(common.Hash{}, inclTx)
					require.Nil(t, got)
					require.ErrorIs(t, err, ethereum.NotFound)
				})
			}
		})
	}
}

func TestGetProof(t *testing.T) {
	someAddress := common.HexToAddress("0xabc")
	blockstore := testutils.NewLocalMemDB(t)

	proofProvider := eth.NewProofProvider(nil, blockstore)

	blockNumber := rpc.LatestBlockNumber
	pf, err := proofProvider.GetProof(context.Background(), someAddress, []string{}, rpc.BlockNumberOrHash{BlockNumber: &blockNumber})
	require.Error(t, err, "should not succeed in generating proofs with empty blockstore")
	require.Nil(t, pf, "received proof from empty blockstore")
}
