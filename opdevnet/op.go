// Package opdevnet contains helpers to build and run an OP devnet, including the L1 but excluding the OP execution engine.
package opdevnet

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-batcher/batcher"
	"github.com/ethereum-optimism/optimism/op-batcher/flags"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	opnodemetrics "github.com/ethereum-optimism/optimism/op-node/metrics"
	opnode "github.com/ethereum-optimism/optimism/op-node/node"
	"github.com/ethereum-optimism/optimism/op-node/rollup/driver"
	"github.com/ethereum-optimism/optimism/op-node/rollup/sync"
	"github.com/ethereum-optimism/optimism/op-proposer/proposer"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
)

type OPConfig struct {
	Proposer *proposer.CLIConfig
	Batcher  *batcher.CLIConfig
	Node     *opnode.Config
}

func BuildOPConfig(
	deployConfig *opgenesis.DeployConfig,
	batcherPrivKey *ecdsa.PrivateKey,
	proposerPrivKey *ecdsa.PrivateKey,
	l1Block *types.Block,
	l2OutputOracleAddr common.Address,
	l2Genesis eth.BlockID,
	l1URL *e2eurl.URL,
	opNodeURL *e2eurl.URL,
	l2EngineURL *e2eurl.URL,
	l2EthURL *e2eurl.URL,
	jwtSecret [32]byte,
) (*OPConfig, error) {
	rollupConfig, err := deployConfig.RollupConfig(l1Block, l2Genesis.Hash, l2Genesis.Number)
	if err != nil {
		return nil, fmt.Errorf("new rollup config: %v", err)
	}

	newTxManagerCLIConfig := func(key *ecdsa.PrivateKey) txmgr.CLIConfig {
		return txmgr.CLIConfig{
			L1RPCURL:                  l1URL.String(),
			PrivateKey:                EncodePrivKeyToString(key),
			NumConfirmations:          1,
			SafeAbortNonceTooLowCount: 3,
			FeeLimitMultiplier:        5,
			ResubmissionTimeout:       3 * time.Second,
			ReceiptQueryInterval:      50 * time.Millisecond,
			NetworkTimeout:            2 * time.Second,
			TxNotInMempoolTimeout:     2 * time.Minute,
		}
	}

	return &OPConfig{
		Node: &opnode.Config{
			L1: &opnode.L1EndpointConfig{
				L1NodeAddr:     l1URL.String(),
				BatchSize:      10,
				MaxConcurrency: 10,
				L1RPCKind:      sources.RPCKindBasic,
			},
			L2: &opnode.L2EndpointConfig{
				L2EngineAddr:      l2EngineURL.String(),
				L2EngineJWTSecret: jwtSecret,
			},
			Driver: driver.Config{
				SequencerEnabled: true,
			},
			Rollup: *rollupConfig,
			RPC: opnode.RPCConfig{
				ListenAddr: opNodeURL.Hostname(),
				ListenPort: int(opNodeURL.PortU16()),
			},
			ConfigPersistence: opnode.DisabledConfigPersistence{},
			Sync: sync.Config{
				SyncMode: sync.CLSync,
			},
		},
		Proposer: &proposer.CLIConfig{
			L1EthRpc:          l1URL.String(),
			RollupRpc:         opNodeURL.String(),
			L2OOAddress:       l2OutputOracleAddr.Hex(),
			PollInterval:      50 * time.Millisecond,
			AllowNonFinalized: true,
			TxMgrConfig:       newTxManagerCLIConfig(proposerPrivKey),
			RPCConfig: oprpc.CLIConfig{
				ListenAddr: "127.0.0.1",
			},
		},
		Batcher: &batcher.CLIConfig{
			L1EthRpc:               l1URL.String(),
			L2EthRpc:               l2EthURL.String(),
			RollupRpc:              opNodeURL.String(),
			SubSafetyMargin:        rollupConfig.SeqWindowSize / 2,
			PollInterval:           50 * time.Millisecond,
			MaxPendingTransactions: 1,
			ApproxComprRatio:       0.4,
			// https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/sequencer_failover_setup.go#L278
			MaxL1TxSize: 120_000,
			TxMgrConfig: newTxManagerCLIConfig(batcherPrivKey),
			RPC: oprpc.CLIConfig{
				ListenAddr: "127.0.0.1",
			},
			DataAvailabilityType: flags.CalldataType,
			TargetNumFrames:      1,
		},
	}, nil
}

// Run may block until it can establish a connection with the Engine, L1, and DA layer.
// Otherwise, it is non-blocking.
func (cfg *OPConfig) Run(ctx context.Context, env *environment.Env, logger log.Logger) error {
	const appVersion = "v0.1"

	// Node
	opNode, err := opnode.New(
		ctx,
		cfg.Node,
		newLogger(logger, "node"),
		newLogger(logger, "node-snapshot"),
		appVersion,
		opnodemetrics.NewMetrics(""),
	)
	if err != nil {
		return fmt.Errorf("new node: %v", err)
	}
	if err := opNode.Start(ctx); err != nil {
		return fmt.Errorf("start node: %v", err)
	}
	env.DeferErr("stop node", func() error {
		return opNode.Stop(context.Background())
	})

	// Proposer
	proposerService, err := proposer.ProposerServiceFromCLIConfig(ctx, "v0.1", cfg.Proposer, newLogger(logger, "proposer"))
	if err != nil {
		return fmt.Errorf("proposer service from CLI config: %v", err)
	}
	if err := proposerService.Start(ctx); err != nil {
		return fmt.Errorf("start proposer service: %v", err)
	}
	env.DeferErr("stop proposer", func() error {
		return proposerService.Stop(context.Background())
	})

	// Batcher
	batcherService, err := batcher.BatcherServiceFromCLIConfig(ctx, "v0.1", cfg.Batcher, newLogger(logger, "batcher"))
	if err != nil {
		return fmt.Errorf("batcher service from CLI config: %v", err)
	}
	if err := batcherService.Start(ctx); err != nil {
		return fmt.Errorf("start batcher service: %v", err)
	}
	env.DeferErr("stop batcher service", func() error {
		// If the environment is closed, the context should be canceled too.
		// That will kill the batcher even if there are in-flight transactions.
		// TODO: perhaps we shouldn't kill the batcher if txs are in-flight? Right now context.Background() lets the batcher hang the test.
		return batcherService.Stop(ctx)
	})

	return nil
}
