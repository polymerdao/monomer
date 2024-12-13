package opdevnet

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-chain-ops/foundry"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/fakebeacon"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/clock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
)

type L1Config struct {
	Genesis      *core.Genesis
	BlobsDirPath string
	BlockTime    uint64
	URL          *e2eurl.URL
	BeaconURL    *e2eurl.URL
}

func BuildL1Config(
	deployConfig *genesis.DeployConfig,
	l1Deployments *genesis.L1Deployments,
	l1Allocs *foundry.ForgeAllocs,
	url *e2eurl.URL,
	beaconURL *e2eurl.URL,
	blobsDirPath string,
) (*L1Config, error) {
	l1Genesis, err := genesis.BuildL1DeveloperGenesis(deployConfig, l1Allocs, l1Deployments)
	if err != nil {
		return nil, fmt.Errorf("build l1 developer genesis: %v", err)
	}
	if scheme := url.Scheme(); scheme != "ws" && scheme != "wss" {
		return nil, fmt.Errorf("l1 url scheme must be ws or wss, got %s", scheme)
	}
	return &L1Config{
		Genesis:      l1Genesis,
		BlobsDirPath: blobsDirPath,
		BlockTime:    deployConfig.L1BlockTime,
		URL:          url,
		BeaconURL:    beaconURL,
	}, nil
}

func (cfg *L1Config) Run(ctx context.Context, env *environment.Env, logger log.Logger) error {
	beacon := fakebeacon.NewBeacon(newLogger(logger, "fakebeacon"), e2eutils.NewBlobStore(), cfg.Genesis.Timestamp, cfg.BlockTime)
	if err := beacon.Start(cfg.BeaconURL.Host()); err != nil {
		return fmt.Errorf("start beacon: %v", err)
	}
	env.Defer(func() {
		// Ignore the error since the close routine is buggy (attempts to close the listener after closing the server).
		_ = beacon.Close()
	})
	gethInstance, err := geth.InitL1(
		cfg.Genesis.Config.ChainID.Uint64(),
		cfg.BlockTime,
		cfg.Genesis,
		clock.NewAdvancingClock(time.Second), // Arbitrary working duration. Eventually consumed by geth lifecycle instances.,
		cfg.BlobsDirPath,
		beacon,
		func(_ *ethconfig.Config, nodeCfg *node.Config) error {
			nodeCfg.WSHost = cfg.URL.Hostname()
			nodeCfg.WSPort = int(cfg.URL.PortU16())
			nodeCfg.HTTPHost = cfg.URL.Hostname()
			nodeCfg.HTTPPort = int(cfg.URL.PortU16())
			nodeCfg.WSOrigins = []string{"*"}
			nodeCfg.Logger = newLogger(logger, "l1")
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("init geth L1: %w", err)
	}

	if err := gethInstance.Node.Start(); err != nil {
		return fmt.Errorf("start geth L1: %w", err)
	}
	env.DeferErr("close geth node", gethInstance.Close)

	return nil
}
