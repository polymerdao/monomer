package contracts

import (
	opcontracts "github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/polymerdao/monomer/bindings/generated"
)

type Predeploy struct {
	address          common.Address
	deployedBytecode []byte
}

var L2ApplicationStateRootProviderAddr = common.HexToAddress("0x4300000000000000000000000000000000000001")

var predeploys = []*Predeploy{
	{
		address:          L2ApplicationStateRootProviderAddr,
		deployedBytecode: common.FromHex(bindings.L2ApplicationStateRootProviderMetaData.Bin),
	},
	{
		address:          opcontracts.L2ToL1MessagePasserAddr,
		deployedBytecode: common.FromHex(bindings.L2ToL1MessagePasserMetaData.Bin),
	},
}

func PredeployContracts(ethState *state.StateDB) *state.StateDB {
	// TODO: investigate using the foundry deploy system for setting up the eth genesis state
	// see https://github.com/polymerdao/monomer/pull/84#discussion_r1697579464
	for _, predeploy := range predeploys {
		ethState.SetCode(predeploy.address, predeploy.deployedBytecode)
	}
	return ethState
}
