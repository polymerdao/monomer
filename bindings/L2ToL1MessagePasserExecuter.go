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
	initiateWithdrawalMethodName = "initiateWithdrawal"
	messageNonceMethodName       = "messageNonce"
	sentMessagesMappingName      = "sentMessages"
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
	sender common.Address,
	amount *big.Int,
	l1Address common.Address,
	gasLimit *big.Int,
	data []byte,
) error {
	data, err := e.ABI.Pack(initiateWithdrawalMethodName, l1Address, gasLimit, data)
	if err != nil {
		return fmt.Errorf("create initiateWithdrawal data: %v", err)
	}

	_, err = e.Call(&monomerevm.CallParams{
		Sender: &sender,
		Value:  amount,
		Data:   data,
	})
	if err != nil {
		return fmt.Errorf("call initiateWithdrawal: %v", err)
	}

	return nil
}

func (e *L2ToL1MessagePasserExecuter) GetSentMessagesMappingValue(withdrawalHash common.Hash) (bool, error) {
	data, err := e.ABI.Pack(sentMessagesMappingName, withdrawalHash)
	if err != nil {
		return false, fmt.Errorf("create sentMessages data: %v", err)
	}

	res, err := e.Call(&monomerevm.CallParams{Data: data})
	if err != nil {
		return false, fmt.Errorf("call sentMessages: %v", err)
	}

	var withdrawalHashIncluded bool
	err = e.ABI.UnpackIntoInterface(&withdrawalHashIncluded, sentMessagesMappingName, res)
	if err != nil {
		return false, fmt.Errorf("unpack sentMessages: %v", err)
	}

	return withdrawalHashIncluded, nil
}

func (e *L2ToL1MessagePasserExecuter) GetMessageNonce() (*big.Int, error) {
	data, err := e.ABI.Pack(messageNonceMethodName)
	if err != nil {
		return nil, fmt.Errorf("create messageNonce data: %v", err)
	}

	res, err := e.Call(&monomerevm.CallParams{Data: data})
	if err != nil {
		return nil, fmt.Errorf("call messageNonce: %v", err)
	}

	var nonce *big.Int
	err = e.ABI.UnpackIntoInterface(&nonce, messageNonceMethodName, res)
	if err != nil {
		return nil, fmt.Errorf("unpack messageNonce: %v", err)
	}

	return nonce, nil
}
