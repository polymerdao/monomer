package e2e

import (
	"fmt"
	"os"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/fakebeacon"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/clock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/environment"
)

func gethdevnet(env *environment.Env, blockTime uint64, genesis *core.Genesis) (*rpc.Client, string, error) {
	blobsDirectory := os.TempDir()

	beacon := fakebeacon.NewBeacon(nil, blobsDirectory, genesis.Timestamp, blockTime)
	myClock := clock.NewAdvancingClock(time.Second) // Arbitrary working duration. Eventually consumed by geth lifecycle instances.
	node, _, err := geth.InitL1(
		genesis.Config.ChainID.Uint64(),
		blockTime,
		genesis,
		myClock,
		blobsDirectory,
		beacon,
	)
	if err != nil {
		return nil, "", fmt.Errorf("init geth L1: %w", err)
	}

	err = node.Start()
	if err != nil {
		return nil, "", fmt.Errorf("start geth L1: %w", err)
	}

	env.DeferErr("close geth node", node.Close)

	return node.Attach(), node.WSEndpoint(), nil
}
