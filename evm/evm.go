package evm

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"math/big"
)

var MonomerEVMChainID = big.NewInt(1)

func NewEVM(ethState vm.StateDB) (*vm.EVM, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}
	return vm.NewEVM(
		vm.BlockContext{
			BlockNumber: big.NewInt(0),
			Transfer:    func(vm.StateDB, common.Address, common.Address, *uint256.Int) {},
		},
		vm.TxContext{
			// TODO: Do we want to use a deterministic address or a random one?
			Origin: crypto.PubkeyToAddress(privateKey.PublicKey),
		},
		ethState,
		&params.ChainConfig{
			ChainID:             MonomerEVMChainID,
			ConstantinopleBlock: big.NewInt(0),
		},
		vm.Config{},
	), nil
}
