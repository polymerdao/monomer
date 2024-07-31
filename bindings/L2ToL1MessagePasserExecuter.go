package bindings

import (
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	bindings "github.com/polymerdao/monomer/bindings/generated"
	monomerevm "github.com/polymerdao/monomer/evm"
)

const (
	InitiateWithdrawalMethodName = "initiateWithdrawal"
	SentMessagesMappingName      = "sentMessages"
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
	data, err := e.ABI.Pack(InitiateWithdrawalMethodName, l1Address, gasLimit, data)
	if err != nil {
		return fmt.Errorf("create initiateWithdrawal data: %v", err)
	}

	senderEthAddress := common.HexToAddress(sender)

	_, err = e.Call(&monomerevm.CallParams{
		Sender: &senderEthAddress,
		Value:  amount,
		Data:   data,
	})
	if err != nil {
		return fmt.Errorf("call initiateWithdrawal: %v", err)
	}

	return nil
}

func (e *L2ToL1MessagePasserExecuter) GetSentMessagesMappingValue(withdrawalHash common.Hash) (bool, error) {
	data, err := e.ABI.Pack(SentMessagesMappingName, withdrawalHash)
	if err != nil {
		return false, fmt.Errorf("create sentMessages data: %v", err)
	}

	res, err := e.Call(&monomerevm.CallParams{Data: data})
	if err != nil {
		return false, fmt.Errorf("call sentMessages: %v", err)
	}

	var withdrawalHashIncluded bool
	err = e.ABI.UnpackIntoInterface(&withdrawalHashIncluded, SentMessagesMappingName, res)
	if err != nil {
		return false, fmt.Errorf("unpack sentMessages: %v", err)
	}

	return withdrawalHashIncluded, nil
}
