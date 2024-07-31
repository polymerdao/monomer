package evm_test

import (
	"math/big"
	"testing"

	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/contracts"
	"github.com/polymerdao/monomer/evm"
	"github.com/polymerdao/monomer/testutils"
	"github.com/stretchr/testify/require"
)

func TestL2ApplicationStateRootProviderExecuter(t *testing.T) {
	executer, err := bindings.NewL2ApplicationStateRootProviderExecuter(setupEVM(t))
	require.NoError(t, err)

	// Get the initial state root
	emptyStateRoot, err := executer.GetL2ApplicationStateRoot()
	require.NoError(t, err)
	require.Equal(t, common.Hash{}, emptyStateRoot)

	// Update the state root
	newStateRoot := common.HexToHash("0x01")
	err = executer.SetL2ApplicationStateRoot(newStateRoot)
	require.NoError(t, err)

	// Get the updated state root
	gotStateRoot, err := executer.GetL2ApplicationStateRoot()
	require.NoError(t, err)
	require.Equal(t, newStateRoot, gotStateRoot)
}

func TestL2ToL1MessagePasserExecuter(t *testing.T) {
	executer, err := bindings.NewL2ToL1MessagePasserExecuter(setupEVM(t))
	require.NoError(t, err)

	cosmosSenderAddr := "cosmosAddr"
	ethSenderAddr := common.HexToAddress(cosmosSenderAddr)
	amount := big.NewInt(500)
	l1TargetAddress := common.HexToAddress("0x12345abcdef")
	gasLimit := big.NewInt(100_000)
	data := []byte("data")

	withdrawalHash, err := crossdomain.NewWithdrawal(
		big.NewInt(1), // expected nonce
		&ethSenderAddr,
		&l1TargetAddress,
		amount,
		gasLimit,
		data,
	).Hash()
	require.NoError(t, err)

	// Check that the withdrawal hash is not in the sentMessages mapping
	sentMessagesMappingValue, err := executer.GetSentMessagesMappingValue(withdrawalHash)
	require.NoError(t, err)
	require.False(t, sentMessagesMappingValue)

	// Initiate a withdrawal
	err = executer.InitiateWithdrawal(cosmosSenderAddr, amount, l1TargetAddress, gasLimit, data)
	require.NoError(t, err)

	// Check that the withdrawal hash is in the sentMessages mapping
	sentMessagesMappingValue, err = executer.GetSentMessagesMappingValue(withdrawalHash)
	require.NoError(t, err)
	require.True(t, sentMessagesMappingValue)
}

func setupEVM(t *testing.T) *vm.EVM {
	ethState, err := state.New(types.EmptyRootHash, testutils.NewEthStateDB(t), nil)
	require.NoError(t, err)
	monomerEVM, err := evm.NewEVM(
		contracts.PredeployContracts(ethState),
		&monomer.Header{
			ChainID: monomer.ChainID(1),
			Height:  1,
		},
	)
	require.NoError(t, err)
	return monomerEVM
}
