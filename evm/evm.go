package evm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"math/big"
)

var (
	// MonomerGenesisRootHash is the known root hash of the monomer ethereum state trie after all predeployed contracts are created.
	MonomerGenesisRootHash = common.HexToHash("0x4e171626bc7f95d0b053dab23c87327ee0266dda88b213a3e1f1357b741c0c35")
	// TODO: is there a specific address we want for MonomerEVMOriginAddress?
	// MonomerEVMOriginAddress is the address used for executing transactions in the monomer EVM.
	MonomerEVMOriginAddress = common.HexToAddress("0xae76d9126c32d7eafe3f5b7bd5a7b44f2d5bb8b1")
)

func NewEVM(ethState vm.StateDB) (*vm.EVM, error) {
	return vm.NewEVM(
		vm.BlockContext{
			BlockNumber: big.NewInt(0),
			Transfer:    func(vm.StateDB, common.Address, common.Address, *uint256.Int) {},
			GasLimit:    10000000,
		},
		vm.TxContext{
			Origin: MonomerEVMOriginAddress,
		},
		ethState,
		&params.ChainConfig{
			ConstantinopleBlock: big.NewInt(0), // Required for the SHR opcode
		},
		vm.Config{},
	), nil
}
