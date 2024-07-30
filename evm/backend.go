package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

type MonomerContractBackend struct {
	statedb *state.StateDB
	evm     *vm.EVM
}

func NewMonomerContractBackend(statedb *state.StateDB, evm *vm.EVM) *MonomerContractBackend {
	return &MonomerContractBackend{
		statedb: statedb,
		evm:     evm,
	}
}

// TODO: should we not use the bindings and try to find a simpler way to execute EVM txs
// since SendTransaction (and CallContext for tests) are the only interface methods we need?
// It would be nice to avoid needing to add tx sigs.
func (b *MonomerContractBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	value, overflow := uint256.FromBig(tx.Value())
	if overflow {
		return errors.New("send transaction value overflow")
	}
	// TODO: do we care about the tx result or leftover gas?
	_, _, err := b.evm.Call(
		vm.AccountRef(b.evm.TxContext.Origin),
		*tx.To(),
		tx.Data(),
		tx.Gas(),
		value,
	)
	return err
}

func (b *MonomerContractBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	res, _, err := b.evm.Call(
		vm.AccountRef(call.From),
		*call.To,
		call.Data,
		10000, // TODO: investigate why this needs gas if it's a call and doesn't modify state
		uint256.NewInt(0),
	)
	if err != nil {
		return nil, fmt.Errorf("call contract: %v", err)
	}
	return res, nil
}

func (b *MonomerContractBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	panic("not implemented")
}

func (b *MonomerContractBackend) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	panic("not implemented")
}
