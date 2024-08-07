package bindings

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/polymerdao/monomer/bindings/generated"
	"github.com/polymerdao/monomer/contracts"
	monomerevm "github.com/polymerdao/monomer/evm"
)

const (
	setL2ApplicationStateRootMethodName = "setL2ApplicationStateRoot"
	getL2ApplicationStateRootMethodName = "l2ApplicationStateRoot"
)

type L2ApplicationStateRootProviderExecuter struct {
	*monomerevm.MonomerContractExecuter
}

func NewL2ApplicationStateRootProviderExecuter(evm *vm.EVM) (*L2ApplicationStateRootProviderExecuter, error) {
	executer, err := monomerevm.NewMonomerContractExecuter(
		evm,
		bindings.L2ApplicationStateRootProviderMetaData.ABI,
		contracts.L2ApplicationStateRootProviderAddr,
	)
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProviderExecuter{executer}, nil
}

func (e *L2ApplicationStateRootProviderExecuter) GetL2ApplicationStateRoot() (common.Hash, error) {
	data, err := e.ABI.Pack(getL2ApplicationStateRootMethodName)
	if err != nil {
		return common.Hash{}, fmt.Errorf("create l2ApplicationStateRoot data: %v", err)
	}

	res, err := e.Call(&monomerevm.CallParams{Data: data})
	if err != nil {
		return common.Hash{}, fmt.Errorf("call getL2ApplicationStateRoot: %v", err)
	}

	var stateRoot common.Hash
	err = e.ABI.UnpackIntoInterface(&stateRoot, getL2ApplicationStateRootMethodName, res)
	if err != nil {
		return common.Hash{}, fmt.Errorf("unpack l2ApplicationStateRoot: %v", err)
	}

	return stateRoot, err
}

func (e *L2ApplicationStateRootProviderExecuter) SetL2ApplicationStateRoot(stateRoot common.Hash) error {
	data, err := e.ABI.Pack(setL2ApplicationStateRootMethodName, stateRoot)
	if err != nil {
		return fmt.Errorf("create setL2ApplicationStateRoot data: %v", err)
	}

	_, err = e.Call(&monomerevm.CallParams{Data: data})
	if err != nil {
		return fmt.Errorf("call setL2ApplicationStateRoot: %v", err)
	}

	return nil
}
