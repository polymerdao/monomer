package bindings

import (
	"bytes"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/solabi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/polymerdao/monomer/bindings/generated"
	"github.com/polymerdao/monomer/contracts"
	monomerevm "github.com/polymerdao/monomer/evm"
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
	data, err := e.ABI.Pack("l2ApplicationStateRoot")
	if err != nil {
		return common.Hash{}, fmt.Errorf("create l2ApplicationStateRoot data: %v", err)
	}

	res, err := e.Call(data, 0)
	if err != nil {
		return common.Hash{}, fmt.Errorf("call getL2ApplicationStateRoot: %v", err)
	}

	stateRoot, err := solabi.ReadEthBytes32(bytes.NewReader(res))
	if err != nil {
		return common.Hash{}, fmt.Errorf("read state root: %v", err)
	}

	return common.Hash(stateRoot), err
}

func (e *L2ApplicationStateRootProviderExecuter) SetL2ApplicationStateRoot(stateRoot common.Hash) error {
	data, err := e.ABI.Pack("setL2ApplicationStateRoot", stateRoot)
	if err != nil {
		return fmt.Errorf("create setL2ApplicationStateRoot data: %v", err)
	}

	_, err = e.Call(data, 0)
	if err != nil {
		return fmt.Errorf("call setL2ApplicationStateRoot: %v", err)
	}

	return nil
}
