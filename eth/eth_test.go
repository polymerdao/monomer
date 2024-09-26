package eth_test

import (
	"context"
	"math/big"
	"testing"

	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
			require.Equal(t, hexID, eth.NewChainIDAPI(hexID, eth.NewNoopMetrics()).ChainId())
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
					blockAPI := eth.NewBlockAPI(blockStore, chainID, eth.NewNoopMetrics())
					got, err := blockAPI.GetBlockByNumber(test.id, fullTxs)
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
				blockAPI := eth.NewBlockAPI(blockStore, chainID, eth.NewNoopMetrics())
				got, err := blockAPI.GetBlockByHash(block.Header.Hash, fullTx)
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
					e := eth.NewBlockAPI(blockStore, chainID, eth.NewNoopMetrics())
					got, err := e.GetBlockByHash(common.Hash{}, inclTx)
					require.Nil(t, got)
					require.ErrorIs(t, err, ethereum.NotFound)
				})
			}
		})
	}
}

// generateProofAPI creates a ProofAPI instance for testing
func generateProofAPI(t *testing.T, blockStoreIsEmpty bool) *eth.ProofAPI {
	t.Helper()
	blockStore := testutils.NewLocalMemDB(t)
	db := testutils.NewEthStateDB(t)

	if !blockStoreIsEmpty {
		block := testutils.GenerateBlock(t)
		require.NoError(t, blockStore.AppendBlock(block))
		require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))
	}

	return eth.NewProofAPI(db, blockStore)
}

func TestGetProof(t *testing.T) {
	blockNumber := rpc.LatestBlockNumber
	zeroHash := common.Hash{}
	zeroHashStr := zeroHash.String()
	someAddress := common.HexToAddress("0xabc")
	zeroBig := (*hexutil.Big)(new(big.Int).SetBytes(zeroHash.Bytes()))

	testCases := []struct {
		name              string
		blockStoreIsEmpty bool
		storageKeys       []string
		expectedResult    *ethapi.AccountResult
		expectError       bool
	}{
		{
			name:              "empty blockstore",
			blockStoreIsEmpty: true,
			expectError:       true,
		},
		{
			name:              "blockstore with block without storageKeys",
			blockStoreIsEmpty: false,
			expectedResult: &ethapi.AccountResult{
				Address:      someAddress,
				AccountProof: nil,
				Balance:      zeroBig,
				CodeHash:     zeroHash,
				Nonce:        hexutil.Uint64(0),
				StorageHash:  zeroHash,
				StorageProof: []ethapi.StorageResult{},
			},
		},
		{
			name:              "blockstore with block with storageKeys and nil storageTrie",
			blockStoreIsEmpty: false,
			storageKeys:       []string{zeroHashStr},
			expectedResult: &ethapi.AccountResult{
				Address:      someAddress,
				AccountProof: nil,
				Balance:      zeroBig,
				CodeHash:     zeroHash,
				Nonce:        hexutil.Uint64(0),
				StorageHash:  zeroHash,
				StorageProof: []ethapi.StorageResult{
					{
						Key:   zeroHashStr,
						Value: &hexutil.Big{},
						Proof: []string{},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proofAPI := generateProofAPI(t, tc.blockStoreIsEmpty)
			pf, err := proofAPI.GetProof(context.Background(), someAddress, tc.storageKeys, rpc.BlockNumberOrHash{BlockNumber: &blockNumber})

			if tc.expectError {
				require.Error(t, err, "should not succeed in generating proofs")
				require.Nil(t, pf, "received proof when error was expected")
			} else {
				require.NoError(t, err, "should succeed in generating proofs")
				require.Equal(t, tc.expectedResult, pf)
			}
		})
	}
}
