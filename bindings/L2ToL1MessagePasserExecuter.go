package bindings

import (
	"fmt"
	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	bindings "github.com/polymerdao/monomer/bindings/generated"
	monomerevm "github.com/polymerdao/monomer/evm"
	"math/big"
)

type L2ToL1MessagePasserExecuter struct {
	*monomerevm.MonomerContractExecuter
}

func NewL2ToL1MessagePasserExecuter(evm *vm.EVM) (*L2ToL1MessagePasserExecuter, error) {
	executer, err := monomerevm.NewMonomerContractExecuter(
		evm,
		bindings.L2ToL1MessagePasserMetaData.ABI,
		predeploys.L2ToL1MessagePasserAddr,
	)
	if err != nil {
		return nil, err
	}
	return &L2ToL1MessagePasserExecuter{executer}, nil
}

func (e *L2ToL1MessagePasserExecuter) InitiateWithdrawal(
	sender string,
	amount *big.Int,
	l1Address common.Address,
	gasLimit *big.Int,
	data []byte,
) error {
	data, err := e.ABI.Pack("initiateWithdrawal", l1Address, gasLimit, data)
	if err != nil {
		return fmt.Errorf("create initiateWithdrawal data: %v", err)
	}

	// TODO: How should we pass through the cosmos sender address to verify that they were the one that withdrew on L2?
	// Do we need to maintain a separate account mapping to ensure that eth msg.sender matches up instead of using the global EVM tx account?
	// TODO: How should we ensure that the msg.value has enough funds to cover the withdrawal? Should we prepopulate an account(s) balance?
	_, err = e.Call(data, 0)
	if err != nil {
		return fmt.Errorf("call initiateWithdrawal: %v", err)
	}

	return nil
}
