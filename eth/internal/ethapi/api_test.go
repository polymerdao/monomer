package ethapi_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

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
	zeroBig := new(hexutil.Big)
	require.NoError(t, zeroBig.UnmarshalText([]byte("0x0")))

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
