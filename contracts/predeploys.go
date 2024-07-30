package contracts

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/bindings"
)

// TODO: should MonomerGenesisRootHash be moved to the evm package?
// TODO: is there a better way to test the genesis? Maybe iterate through the predeploys and check if the address has the correct bytecode?
// MonomerGenesisRootHash is the known root hash of the monomer ethereum state trie after all predeployed contracts are created.
var MonomerGenesisRootHash = common.HexToHash("0x4e171626bc7f95d0b053dab23c87327ee0266dda88b213a3e1f1357b741c0c35")

type Predeploy struct {
	Address          common.Address
	DeployedBytecode []byte
}

// TODO: is there a specific address we want L2ApplicationStateRootProviderAddr to be?
var L2ApplicationStateRootProviderAddr = common.HexToAddress("0x4e7a96d48e79e61c7aee5ef9e59d7cfc6f0bdc8d")

var Predeploys = []*Predeploy{
	{
		Address:          L2ApplicationStateRootProviderAddr,
		DeployedBytecode: common.FromHex(bindings.L2ApplicationStateRootProviderMetaData.Bin),
	},
}
