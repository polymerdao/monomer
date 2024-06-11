package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/fakebeacon"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/clock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rpc"
)

func ethdevnet(_ context.Context, chainid uint64, blockTime uint64, genesis *core.Genesis) *rpc.Client {
	now := time.Now().Unix()

	tmpDir := "~/tmp"

	// logger := log.New(log.Writer(), "ethdevnet", log.LstdFlags)

	beacon := fakebeacon.NewBeacon(nil, tmpDir, uint64(now), blockTime)
	// geth.InitL1(0, 0, &core.Genesis{}, clock.NewSimpleClock(), t.TempDir())

	myClock := clock.NewAdvancingClock(time.Millisecond * 500)
	node, client, err := geth.InitL1(chainid, blockTime, genesis, myClock, tmpDir, beacon)

	fmt.Println("node service at: ", node.HTTPEndpoint())

	fmt.Println(client)

	if err != nil {
		panic(err)
	}
	return node.Attach()
}
