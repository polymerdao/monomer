package eth_test

import (
	"context"
	"math/big"
	"testing"

	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
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

func TestGetProof(t *testing.T) {
	blockNumber := rpc.LatestBlockNumber
	zeroHash := common.Hash{}
	zeroHashStr := zeroHash.String()
	accountAddress := common.HexToAddress("0xabc")
	zeroBig := (*hexutil.Big)(new(big.Int).SetBytes(zeroHash.Bytes()))

	testCases := []struct {
		name              string
		blockStoreIsEmpty bool
		ethStateIsEmpty   bool
		storageKeys       []string
		expectedResult    *ethapi.AccountResult
		expectError       bool
	}{
		{
			name:              "empty blockstore and eth state",
			blockStoreIsEmpty: true,
			ethStateIsEmpty:   true,
			expectError:       true,
		},
		{
			name:              "empty eth state and blockstore with block without storageKeys",
			blockStoreIsEmpty: false,
			expectedResult: &ethapi.AccountResult{
				Address:      accountAddress,
				AccountProof: nil,
				Balance:      zeroBig,
				CodeHash:     zeroHash,
				Nonce:        hexutil.Uint64(0),
				StorageHash:  zeroHash,
				StorageProof: []ethapi.StorageResult{},
			},
		},
		{
			name:              "empty eth state and blockstore with block with storageKeys and nil storageTrie",
			blockStoreIsEmpty: false,
			storageKeys:       []string{zeroHashStr},
			expectedResult: &ethapi.AccountResult{
				Address:      accountAddress,
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
		{
			name:              "populated eth state and blockstore",
			blockStoreIsEmpty: false,
			storageKeys:       []string{zeroHashStr},
			expectedResult: &ethapi.AccountResult{
				Address:      accountAddress,
				AccountProof: nil,
				// TODO: extract vars for balance, nonce, etc?
				Balance:     (*hexutil.Big)(big.NewInt(1000)),
				CodeHash:    crypto.Keccak256Hash([]byte{1, 2, 3}),
				Nonce:       hexutil.Uint64(1),
				StorageHash: zeroHash,
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
			blockStore := testutils.NewLocalMemDB(t)
			ethStateDB := testutils.NewEthStateDB(t)

			var stateRoot common.Hash

			if !tc.ethStateIsEmpty {
				ethState, err := state.New(ethtypes.EmptyRootHash, ethStateDB, nil)
				require.NoError(t, err)

				ethState.SetNonce(accountAddress, 1)
				ethState.SetBalance(accountAddress, uint256.NewInt(1000))
				ethState.SetCode(accountAddress, []byte{1, 2, 3})
				//ethState.SetStorage() TODO: set storage

				stateRoot, err = ethState.Commit(uint64(blockNumber.Int64()), true)
				require.NoError(t, err)

				err = ethState.Database().TrieDB().Commit(stateRoot, false)
				require.NoError(t, err)
			} else {
				stateRoot = ethtypes.EmptyRootHash
			}

			if !tc.blockStoreIsEmpty {
				block := testutils.GenerateBlock(t)
				block.Header.StateRoot = stateRoot
				require.NoError(t, blockStore.AppendBlock(block))
				require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))
			}

			proofAPI := eth.NewProofAPI(ethStateDB, blockStore)
			pf, err := proofAPI.GetProof(context.Background(), accountAddress, tc.storageKeys, rpc.BlockNumberOrHash{BlockNumber: &blockNumber})

			if tc.expectError {
				require.Error(t, err, "should not succeed in generating proofs")
				require.Nil(t, pf, "received proof when error was expected")
			} else {
				// TODO: this check may not be necessary if we're using verify proof. It may be nice asserting that the account proof/storage proof are populated when expected though
				require.NoError(t, err, "should succeed in generating proofs")
				require.Equal(t, tc.expectedResult, pf)

				// TODO: verify proof here
				//err = withdrawals.VerifyProof(stateRoot, tc.expectedResult)
			}
		})
	}
}
