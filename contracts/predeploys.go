package contracts

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/bindings/generated"
)

type Predeploy struct {
	Address          common.Address
	DeployedBytecode []byte
}

var L2ApplicationStateRootProviderAddr = common.HexToAddress("0x4e7a96d48e79e61c7aee5ef9e59d7cfc6f0bdc8d")

var Predeploys = []*Predeploy{
	{
		Address:          L2ApplicationStateRootProviderAddr,
		DeployedBytecode: common.FromHex(bindings.L2ApplicationStateRootProviderMetaData.Bin),
	},
}
