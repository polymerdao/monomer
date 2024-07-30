package bindings

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymerdao/monomer/evm"
)

func NewTransactOpts() (*bind.TransactOpts, error) {
	// TODO: Should we use a deterministic private key? Also combine this with the EVM origin address and store as a constant
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}
	authOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, evm.MonomerEVMChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transact opts: %v", err)
	}
	return &bind.TransactOpts{
		From:      authOpts.From,
		Nonce:     big.NewInt(1),
		Signer:    authOpts.Signer,
		Value:     nil,
		GasPrice:  big.NewInt(0),
		GasFeeCap: nil,
		GasTipCap: nil,
		GasLimit:  100000,
		Context:   authOpts.Context,
		NoSend:    false,
	}, nil
}
