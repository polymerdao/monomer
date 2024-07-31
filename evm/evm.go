package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/core"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/utils"
)

var (
	// MonomerGenesisRootHash is the known root hash of the monomer ethereum state trie after all predeployed contracts are created.
	MonomerGenesisRootHash = common.HexToHash("0x5be0a68aae2d389cd9c9276ece59f483b97da7e99d2ff157923f4822dc107b6b")
	// MonomerEVMTxOriginAddress is the address used for executing transactions in the monomer evm.
	MonomerEVMTxOriginAddress = common.HexToAddress("0x4300000000000000000000000000000000000000")
)

func NewEVM(ethState vm.StateDB, header *monomer.Header, chainID *big.Int) (*vm.EVM, error) {
	chainConfig := &params.ChainConfig{
		ChainID: chainID,

		ByzantiumBlock:      new(big.Int),
		ConstantinopleBlock: new(big.Int),
		PetersburgBlock:     new(big.Int),
		IstanbulBlock:       new(big.Int),
		MuirGlacierBlock:    new(big.Int),
		// TODO: investigate SSTORE access list evm execution bug with BerlinBlock/LondonBlock
		// BerlinBlock:        new(big.Int),
		// LondonBlock:        new(big.Int),
		ArrowGlacierBlock:  new(big.Int),
		GrayGlacierBlock:   new(big.Int),
		MergeNetsplitBlock: new(big.Int),

		BedrockBlock: new(big.Int),
		RegolithTime: utils.Ptr(uint64(0)),
		CanyonTime:   utils.Ptr(uint64(0)),
	}
	blockContext := core.NewEVMBlockContext(header.ToEth(), mockChainContext{}, &MonomerEVMTxOriginAddress, chainConfig, ethState)
	// TODO: investigate having an unlimited gas limit for monomer evm execution
	blockContext.GasLimit = 100_000_000

	return vm.NewEVM(
		blockContext,
		vm.TxContext{
			Origin:   MonomerEVMTxOriginAddress,
			GasPrice: big.NewInt(0),
		},
		ethState,
		chainConfig,
		vm.Config{
			NoBaseFee: true,
		},
	), nil
}

type mockChainContext struct{}

func (c mockChainContext) Engine() consensus.Engine {
	return beacon.NewFaker()
}

func (c mockChainContext) GetHeader(common.Hash, uint64) *gethtypes.Header {
	panic("not implemented")
}
