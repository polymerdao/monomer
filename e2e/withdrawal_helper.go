package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/indexer/bindings"
	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum-optimism/optimism/op-node/withdrawals"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ProvenWithdrawalParameters is the set of parameters to pass to the ProveWithdrawalTransaction
// and FinalizeWithdrawalTransaction functions
type ProvenWithdrawalParameters struct {
	Nonce           *big.Int
	Sender          common.Address
	Target          common.Address
	Value           *big.Int
	GasLimit        *big.Int
	L2OutputIndex   *big.Int
	Data            []byte
	OutputRootProof bindings.TypesOutputRootProof
	WithdrawalProof [][]byte // List of trie nodes to prove L2 storage
}

// TODO: add documentation
// TODO: add note and link to the code that inspired this function
func ProveWithdrawalParameters(ctx context.Context, stack *StackConfig, withdrawalTx crossdomain.Withdrawal, l2BlockNumber *big.Int, l2OutputOracleContract *bindings.L2OutputOracleCaller) (ProvenWithdrawalParameters, error) {
	l2OutputIndex, err := l2OutputOracleContract.GetL2OutputIndexAfter(&bind.CallOpts{}, l2BlockNumber)
	if err != nil {
		return ProvenWithdrawalParameters{}, fmt.Errorf("failed to get l2OutputIndex: %w", err)
	}

	// Generate then verify the withdrawal proof
	withdrawalHash, err := withdrawalTx.Hash()
	if err != nil {
		return ProvenWithdrawalParameters{}, err
	}

	// Fetch the block from the monomer client
	l2Block, err := stack.MonomerClient.BlockByNumber(ctx, l2BlockNumber)
	if err != nil {
		return ProvenWithdrawalParameters{}, fmt.Errorf("failed to fetch block %v from the monomer client: %w", l2BlockNumber, err)
	}

	proof, err := stack.MonomerClient.GetProof(ctx, predeploys.L2ToL1MessagePasserAddr, []string{storageSlotOfWithdrawalHash(withdrawalHash).String()}, l2Block.Number())
	if err != nil {
		return ProvenWithdrawalParameters{}, err
	}
	if len(proof.StorageProof) != 1 {
		return ProvenWithdrawalParameters{}, errors.New("invalid amount of storage proofs")
	}

	err = withdrawals.VerifyProof(l2Block.Root(), proof)
	if err != nil {
		return ProvenWithdrawalParameters{}, err
	}

	// Encode it as expected by the contract
	trieNodes := make([][]byte, len(proof.StorageProof[0].Proof))
	for i, s := range proof.StorageProof[0].Proof {
		trieNodes[i] = common.FromHex(s)
	}

	return ProvenWithdrawalParameters{
		Nonce:         withdrawalTx.Nonce,
		Sender:        *withdrawalTx.Sender,
		Target:        *withdrawalTx.Target,
		Value:         withdrawalTx.Value,
		GasLimit:      withdrawalTx.GasLimit,
		L2OutputIndex: l2OutputIndex,
		Data:          withdrawalTx.Data,
		OutputRootProof: bindings.TypesOutputRootProof{
			Version:                  [32]byte{}, // Empty for version 1
			StateRoot:                l2Block.Root(),
			MessagePasserStorageRoot: proof.StorageHash,
			LatestBlockhash:          l2Block.Hash(),
		},
		WithdrawalProof: trieNodes,
	}, nil
}

func NewWithdrawalTx(nonce int64, sender, target common.Address, value *big.Int) *crossdomain.Withdrawal {
	return &crossdomain.Withdrawal{
		Nonce:  crossdomain.EncodeVersionedNonce(big.NewInt(nonce), big.NewInt(1)),
		Sender: &sender,
		Target: &target,
		Value:  value,
		// TODO: make gas limit configurable?
		GasLimit: big.NewInt(100_000),
		Data:     []byte{},
	}
}

// storageSlotOfWithdrawalHash determines the storage slot of the L2ToL1MessagePasser contract to look at
// given a WithdrawalHash
func storageSlotOfWithdrawalHash(hash common.Hash) common.Hash {
	// The withdrawals mapping is the 0th storage slot in the L2ToL1MessagePasser contract.
	// To determine the storage slot, use keccak256(withdrawalHash ++ p)
	// Where p is the 32 byte value of the storage slot and ++ is concatenation
	buf := make([]byte, 64)
	copy(buf, hash[:])
	return crypto.Keccak256Hash(buf)
}
