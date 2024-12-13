package opdevnet

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-chain-ops/foundry"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var (
	//go:embed config/addresses.json
	l1DeploymentsJSON []byte
	//go:embed config/devnetL1.json
	deployConfigJSON []byte
	//go:embed config/allocs-l1.json
	l1AllocsJSON []byte
	//go:embed config/allocs-l2-granite.json
	l2AllocsJSON []byte
)

func DefaultL1Deployments() (*genesis.L1Deployments, error) {
	var l1Deployments genesis.L1Deployments
	if err := json.Unmarshal(l1DeploymentsJSON, &l1Deployments); err != nil {
		return nil, fmt.Errorf("unmarshal l1 deployments: %v", err)
	}
	return &l1Deployments, nil
}

func DefaultDeployConfig(l1Deployments *genesis.L1Deployments) (*genesis.DeployConfig, error) {
	var deployConfig genesis.DeployConfig
	if err := json.Unmarshal(deployConfigJSON, &deployConfig); err != nil {
		return nil, fmt.Errorf("unmarshal deploy config: %v", err)
	}

	// See https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/config/init.go#L138-L150

	// Do not use clique in the in memory tests. Otherwise block building would be much more complex.
	deployConfig.L1UseClique = false
	// Set the L1 genesis block timestamp to now
	deployConfig.L1GenesisBlockTimestamp = hexutil.Uint64(time.Now().Unix())
	deployConfig.FundDevAccounts = true
	// Speed up the in memory tests
	deployConfig.L1BlockTime = 2
	deployConfig.L2BlockTime = 1
	// Set the L1 deployments
	deployConfig.SetDeployments(l1Deployments)

	// Set a shorter Sequencer Window Size to force unsafe block consolidation to happen more often.
	// A verifier (and the sequencer when it's determining the safe head) will have to read the entire sequencer window
	// before advancing in the worst case. For the sake of tests running quickly, we minimize that worst case to 4 blocks.
	deployConfig.SequencerWindowSize = 4

	return &deployConfig, nil
}

func DefaultL1Allocs() (*foundry.ForgeAllocs, error) {
	var l1Allocs foundry.ForgeAllocs
	if err := json.Unmarshal(l1AllocsJSON, &l1Allocs); err != nil {
		return nil, fmt.Errorf("cannot unmarshal dump: %w", err)
	}
	return &l1Allocs, nil
}

func DefaultL2Allocs() (*foundry.ForgeAllocs, error) {
	var l2Allocs foundry.ForgeAllocs
	if err := json.Unmarshal(l2AllocsJSON, &l2Allocs); err != nil {
		return nil, fmt.Errorf("unmarshal l2 allocs: %v", err)
	}
	return &l2Allocs, nil
}
