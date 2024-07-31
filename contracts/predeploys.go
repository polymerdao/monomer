package contracts

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/bindings/generated"
)

type Predeploy struct {
	Address          common.Address
	DeployedBytecode []byte
}

var L2ApplicationStateRootProviderAddr = common.HexToAddress("0x4300000000000000000000000000000000000001")

var Predeploys = []*Predeploy{
	{
		Address:          L2ApplicationStateRootProviderAddr,
		DeployedBytecode: common.FromHex(bindings.L2ApplicationStateRootProviderMetaData.Bin),
	},
}
