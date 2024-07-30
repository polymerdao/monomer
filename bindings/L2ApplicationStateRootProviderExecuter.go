package bindings

import (
	"fmt"
	"strings"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
	"github.com/polymerdao/monomer/bindings/generated"
	"github.com/polymerdao/monomer/contracts"
)

type L2ApplicationStateRootProviderExecuter struct {
	abi          *gethabi.ABI
	evm          *vm.EVM
	contractAddr common.Address
}

func NewL2ApplicationStateRootProviderExecuter(evm *vm.EVM) (*L2ApplicationStateRootProviderExecuter, error) {
	abi, err := gethabi.JSON(strings.NewReader(bindings.L2ApplicationStateRootProviderMetaData.ABI))
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProviderExecuter{
		abi:          &abi,
		evm:          evm,
		contractAddr: contracts.L2ApplicationStateRootProviderAddr,
	}, nil
}

func (e *L2ApplicationStateRootProviderExecuter) GetL2ApplicationStateRoot() ([32]byte, error) {
	data, err := e.abi.Pack("l2ApplicationStateRoot")
	if err != nil {
		return [32]byte{}, fmt.Errorf("create l2ApplicationStateRoot data: %v", err)
	}

	res, _, err := e.evm.Call(
		vm.AccountRef(e.evm.TxContext.Origin),
		e.contractAddr,
		data,
		e.evm.Context.GasLimit,
		uint256.NewInt(0),
	)
	if err != nil {
		return [32]byte{}, fmt.Errorf("call getL2ApplicationStateRoot: %v", err)
	}

	// TODO: are there existing ABI funcs that can do this conversion from the call result for us?
	var stateRoot [32]byte
	copy(stateRoot[:], res[:32])

	return stateRoot, err
}

func (e *L2ApplicationStateRootProviderExecuter) SetL2ApplicationStateRoot(stateRoot [32]byte) error {
	data, err := e.abi.Pack("setL2ApplicationStateRoot", stateRoot)
	if err != nil {
		return fmt.Errorf("create setL2ApplicationStateRoot data: %v", err)
	}

	_, _, err = e.evm.Call(
		vm.AccountRef(e.evm.TxContext.Origin),
		e.contractAddr,
		data,
		e.evm.Context.GasLimit,
		uint256.NewInt(0),
	)
	if err != nil {
		return fmt.Errorf("call setL2ApplicationStateRoot: %v", err)
	}

	return nil
}
