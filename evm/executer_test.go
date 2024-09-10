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

	sender := common.HexToAddress("0xabcdef12345")
	amount := big.NewInt(500)
	l1TargetAddress := common.HexToAddress("0x12345abcdef")
	gasLimit := big.NewInt(100_000)
	data := []byte("data")
	nonce := encodeVersionedNonce(big.NewInt(0))

	withdrawalHash, err := crossdomain.NewWithdrawal(
		nonce,
		&sender,
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

	// Check that the initial message nonce is 0
	initialMessageNonce, err := executer.GetMessageNonce()
	require.NoError(t, err)
	require.Equal(t, nonce, initialMessageNonce)

	// Initiate a withdrawal
	err = executer.InitiateWithdrawal(sender, amount, l1TargetAddress, gasLimit, data)
	require.NoError(t, err)

	// Check that the withdrawal hash is in the sentMessages mapping
	sentMessagesMappingValue, err = executer.GetSentMessagesMappingValue(withdrawalHash)
	require.NoError(t, err)
	require.True(t, sentMessagesMappingValue)

	// Check that the message nonce is incremented
	messageNonce, err := executer.GetMessageNonce()
	require.NoError(t, err)
	require.Equal(t, encodeVersionedNonce(big.NewInt(1)), messageNonce)
}

func setupEVM(t *testing.T) *vm.EVM {
	ethState, err := state.New(types.EmptyRootHash, testutils.NewEthStateDB(t), nil)
	require.NoError(t, err)
	monomerEVM, err := evm.NewEVM(
		contracts.Predeploy(ethState),
		&monomer.Header{
			ChainID: monomer.ChainID(1),
			Height:  1,
		},
	)
	require.NoError(t, err)
	return monomerEVM
}

func encodeVersionedNonce(nonce *big.Int) *big.Int {
	return crossdomain.EncodeVersionedNonce(nonce, big.NewInt(1))
}
