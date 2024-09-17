package e2e

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum-optimism/optimism/op-node/bindings"
	"github.com/ethereum-optimism/optimism/op-node/withdrawals"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ProveWithdrawalParameters queries L1 & L2 to generate all withdrawal parameters and proof necessary to prove a withdrawal on L1.
// The l2BlockNumber provided is very important. It should be a block that is greater than or equal to the block where the withdrawal
// was initiated on L2 and needs to have a submitted output in the L2 Output Oracle contract. If not, the withdrawal will fail since
// the storage proof cannot be verified if there is no submitted state root.
//
// For example, if a withdrawal was initiated on L2 block 7 and the proposer submits an L2 output to L1 every 5 L2 blocks, then the
// L2 block number to prove against for the withdrawal would need to be in the following subset: (10, 15, 20, 25, ...)
//
// ProveWithdrawalParameters is heavily inspired by ProveWithdrawalParametersForBlock in the optimism repo.
// https://github.com/ethereum-optimism/optimism/blob/5b13bad/op-node/withdrawals/utils.go#L75
func ProveWithdrawalParameters(
	stack *StackConfig,
	withdrawalTx crossdomain.Withdrawal,
	l2BlockNumber *big.Int,
) (withdrawals.ProvenWithdrawalParameters, error) {
	l2OutputIndex, err := stack.L2OutputOracleCaller.GetL2OutputIndexAfter(&bind.CallOpts{}, l2BlockNumber)
	if err != nil {
		return withdrawals.ProvenWithdrawalParameters{}, fmt.Errorf("failed to get l2OutputIndex: %w", err)
	}

	withdrawalHash, err := withdrawalTx.Hash()
	if err != nil {
		return withdrawals.ProvenWithdrawalParameters{}, err
	}

	// Fetch the block from the monomer client
	l2Block, err := stack.MonomerClient.BlockByNumber(stack.Ctx, l2BlockNumber)
	if err != nil {
		return withdrawals.ProvenWithdrawalParameters{}, fmt.Errorf("failed to fetch block %v from the monomer client: %w", l2BlockNumber, err)
	}

	// Generate the withdrawal proof
	proof, err := stack.MonomerClient.GetProof(
		stack.Ctx,
		predeploys.L2ToL1MessagePasserAddr,
		[]string{storageSlotOfWithdrawalHash(withdrawalHash).String()},
		l2Block.Number(),
	)
	if err != nil {
		return withdrawals.ProvenWithdrawalParameters{}, err
	}
	if len(proof.StorageProof) != 1 {
		return withdrawals.ProvenWithdrawalParameters{}, errors.New("invalid amount of storage proofs")
	}

	// Verify the withdrawal proof was generated correctly
	err = withdrawals.VerifyProof(l2Block.Root(), proof)
	if err != nil {
		return withdrawals.ProvenWithdrawalParameters{}, err
	}

	// Encode the withdrawal proof as expected by the contract
	trieNodes := make([][]byte, len(proof.StorageProof[0].Proof))
	for i, s := range proof.StorageProof[0].Proof {
		trieNodes[i] = common.FromHex(s)
	}

	return withdrawals.ProvenWithdrawalParameters{
		L2OutputIndex: l2OutputIndex,
		OutputRootProof: bindings.TypesOutputRootProof{
			Version:                  [32]byte{}, // Empty for version 1
			StateRoot:                l2Block.Root(),
			MessagePasserStorageRoot: proof.StorageHash,
			LatestBlockhash:          l2Block.Hash(),
		},
		WithdrawalProof: trieNodes,
	}, nil
}

// storageSlotOfWithdrawalHash determines the storage slot of the L2ToL1MessagePasser contract to look at
// given a WithdrawalHash
//
// https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html#mappings-and-dynamic-arrays
func storageSlotOfWithdrawalHash(hash common.Hash) common.Hash {
	// The withdrawals mapping is the 0th storage slot in the L2ToL1MessagePasser contract.
	// To determine the storage slot for the given hash key, use keccak256(withdrawalHash ++ p)
	// Where p is the 32 byte value of the storage slot and ++ is concatenation
	buf := make([]byte, 64) //nolint:mnd
	copy(buf, hash[:])
	return crypto.Keccak256Hash(buf)
}

func NewWithdrawalTx(nonce int64, sender, target common.Address, value, gasLimit *big.Int) *crossdomain.Withdrawal {
	return &crossdomain.Withdrawal{
		Nonce:    crossdomain.EncodeVersionedNonce(big.NewInt(nonce), big.NewInt(1)),
		Sender:   &sender,
		Target:   &target,
		Value:    value,
		GasLimit: gasLimit,
		Data:     []byte{},
	}
}
