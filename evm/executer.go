package evm

import (
	"fmt"
	"math/big"
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

type CallParams struct {
	Sender *common.Address
	Value  *big.Int
	Data   []byte
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

func (e *MonomerContractExecuter) Call(params *CallParams) ([]byte, error) {
	sender, value, data := params.getCallParams()
	res, _, err := e.evm.Call(
		vm.AccountRef(sender),
		e.contractAddr,
		data,
		e.evm.Context.GasLimit,
		value,
	)
	if err != nil {
		return nil, fmt.Errorf("call contract %v: %v", e.contractAddr, err)
	}
	return res, nil
}

func (params *CallParams) getCallParams() (common.Address, *uint256.Int, []byte) {
	if params.Sender == nil {
		params.Sender = &MonomerEVMTxOriginAddress
	}
	uint256Value := &uint256.Int{}
	if params.Value == nil {
		uint256Value = uint256.NewInt(0)
	} else {
		uint256Value.SetFromBig(params.Value)
	}
	return *params.Sender, uint256Value, params.Data
}
