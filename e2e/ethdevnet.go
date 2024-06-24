package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/fakebeacon"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/clock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rpc"
)

func ethdevnet(_ context.Context, blockTime uint64, genesis *core.Genesis) (*rpc.Client, string) {
	now := time.Now().Unix()
	blobsDirectory := filepath.Join("artifacts", "blobs")

	beacon := fakebeacon.NewBeacon(nil, blobsDirectory, uint64(now), blockTime)
	myClock := clock.NewAdvancingClock(time.Second / 2)
	node, _, err := geth.InitL1(
		genesis.Config.ChainID.Uint64(),
		blockTime,
		genesis,
		myClock,
		blobsDirectory,
		beacon,
	)
	if err != nil {
		panic(fmt.Errorf("init geth L1: %w", err))
	}

	err = node.Start()
	if err != nil {
		panic(fmt.Errorf("start geth L1: %w", err))
	}

	return node.Attach(), node.WSEndpoint()
}
