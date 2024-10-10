package opdevnet

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/fakebeacon"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/clock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
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
}

func BuildL1Config(
	deployConfig *genesis.DeployConfig,
	l1Deployments *genesis.L1Deployments,
	l1Allocs *state.Dump,
	url *e2eurl.URL,
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
	}, nil
}

func (cfg *L1Config) Run(ctx context.Context, env *environment.Env, logger log.Logger) error {
	l1Node, _, err := geth.InitL1(
		cfg.Genesis.Config.ChainID.Uint64(),
		cfg.BlockTime,
		cfg.Genesis,
		clock.NewAdvancingClock(time.Second), // Arbitrary working duration. Eventually consumed by geth lifecycle instances.,
		cfg.BlobsDirPath,
		fakebeacon.NewBeacon(newLogger(logger, "fakebeacon"), cfg.BlobsDirPath, cfg.Genesis.Timestamp, cfg.BlockTime),
		func(_ *ethconfig.Config, nodeCfg *node.Config) error {
			nodeCfg.WSHost = cfg.URL.Hostname()
			nodeCfg.WSPort = int(cfg.URL.PortU16())
			// TODO: make L1 http port configurable
			nodeCfg.HTTPPort = 39473
			nodeCfg.WSOrigins = []string{"*"}
			nodeCfg.Logger = newLogger(logger, "l1")
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("init geth L1: %w", err)
	}

	if err := l1Node.Start(); err != nil {
		return fmt.Errorf("start geth L1: %w", err)
	}
	env.DeferErr("close geth node", l1Node.Close)

	return nil
}
