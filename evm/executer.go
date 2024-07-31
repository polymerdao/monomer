package evm

import (
	"fmt"
	"strings"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

type MonomerContractExecuter struct {
	ABI          *gethabi.ABI
	evm          *vm.EVM
	contractAddr common.Address
}

func NewMonomerContractExecuter(evm *vm.EVM, abiJSON string, contractAddr common.Address) (*MonomerContractExecuter, error) {
	abi, err := gethabi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &MonomerContractExecuter{
		ABI:          &abi,
		evm:          evm,
		contractAddr: contractAddr,
	}, nil
}

// TODO: use a call struct param to specify the call specific call params or revert to a default if nil
func (e *MonomerContractExecuter) Call(data []byte, value uint64) ([]byte, error) {
	res, _, err := e.evm.Call(
		vm.AccountRef(e.evm.TxContext.Origin),
		e.contractAddr,
		data,
		e.evm.Context.GasLimit,
		uint256.NewInt(value),
	)
	if err != nil {
		return nil, fmt.Errorf("call contract %v: %v", e.contractAddr, err)
	}
	return res, nil
}
