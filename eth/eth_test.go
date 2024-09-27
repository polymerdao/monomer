package eth_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/withdrawals"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
	"github.com/polymerdao/monomer/monomerdb/localdb"
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
	accountAddress := common.HexToAddress("0xabc")

	testCases := []struct {
		name              string
		blockStoreIsEmpty bool
		ethStateIsEmpty   bool
		storageKey        common.Hash
		expectError       bool
	}{
		{
			name:              "empty blockstore and eth state",
			blockStoreIsEmpty: true,
			ethStateIsEmpty:   true,
			expectError:       true,
		},
		{
			name:              "empty eth state and blockstore with block without storageKey",
			blockStoreIsEmpty: false,
			ethStateIsEmpty:   true,
		},
		{
			name:              "empty eth state and blockstore with block with storageKey and nil storageTrie",
			blockStoreIsEmpty: false,
			ethStateIsEmpty:   true,
			storageKey:        common.Hash{},
		},
		{
			name:              "populated eth state and blockstore",
			blockStoreIsEmpty: false,
			ethStateIsEmpty:   false,
			storageKey:        common.HexToHash("0x123"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blockStore := testutils.NewLocalMemDB(t)
			ethStateDB := testutils.NewEthStateDB(t)

			stateRoot := setupEthState(t, ethStateDB, accountAddress, blockNumber, tc.storageKey, tc.ethStateIsEmpty)

			if !tc.blockStoreIsEmpty {
				setupBlockStore(t, blockStore, stateRoot)
			}

			proofAPI := eth.NewProofAPI(ethStateDB, blockStore)
			pf, err := proofAPI.GetProof(context.Background(), accountAddress, []string{tc.storageKey.String()}, rpc.BlockNumberOrHash{BlockNumber: &blockNumber})

			// Verify the expected proof
			if tc.expectError {
				require.Error(t, err, "should not succeed in generating proofs")
				require.Nil(t, pf, "received proof when error was expected")
			} else {
				require.NoError(t, err, "should succeed in generating proofs")
				if tc.ethStateIsEmpty {
					require.NotNil(t, pf, "proof should not be nil")
				} else {
					require.NoError(t, withdrawals.VerifyProof(stateRoot, adaptProof(pf)), "proof verification failed")
				}
			}
		})
	}
}

func setupEthState(t *testing.T, ethStateDB state.Database, accountAddress common.Address, blockNumber rpc.BlockNumber, storageKey common.Hash, ethStateIsEmpty bool) common.Hash {
	if ethStateIsEmpty {
		return ethtypes.EmptyRootHash
	}

	ethState, err := state.New(ethtypes.EmptyRootHash, ethStateDB, nil)
	require.NoError(t, err)

	ethState.SetNonce(accountAddress, 1)
	ethState.SetBalance(accountAddress, uint256.NewInt(1000))
	ethState.SetCode(accountAddress, []byte{1, 2, 3})

	// RLP encode the storage value before setting it in the account state
	storageValue, err := rlp.EncodeToBytes(big.NewInt(100))
	require.NoError(t, err)
	ethState.SetState(accountAddress, storageKey, common.BytesToHash(storageValue))

	// Commit the updated state trie to the database
	stateRoot, err := ethState.Commit(uint64(blockNumber.Int64()), true)
	require.NoError(t, err)
	err = ethState.Database().TrieDB().Commit(stateRoot, false)
	require.NoError(t, err)

	return stateRoot
}

func setupBlockStore(t *testing.T, blockStore *localdb.DB, stateRoot common.Hash) {
	block := testutils.GenerateBlock(t)
	block.Header.StateRoot = stateRoot
	require.NoError(t, blockStore.AppendBlock(block))
	require.NoError(t, blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash))
}

// adaptProof adapts the generated proof from ethapi.AccountResult to gethclient.AccountResult to use with withdrawals.VerifyProof
func adaptProof(pf *ethapi.AccountResult) *gethclient.AccountResult {
	storageProof := make([]gethclient.StorageResult, len(pf.StorageProof))
	for i, sp := range pf.StorageProof {
		storageProof[i] = gethclient.StorageResult{
			Key:   sp.Key,
			Value: sp.Value.ToInt(),
			Proof: sp.Proof,
		}
	}

	return &gethclient.AccountResult{
		Address:      pf.Address,
		AccountProof: pf.AccountProof,
		Balance:      pf.Balance.ToInt(),
		CodeHash:     pf.CodeHash,
		Nonce:        uint64(pf.Nonce),
		StorageHash:  pf.StorageHash,
		StorageProof: storageProof,
	}
}
