package opdevnet_test

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/stretchr/testify/require"
)

const artifactsDirectoryName = "artifacts"

func openLogFile(t *testing.T, env *environment.Env, name string) *os.File {
	filename := filepath.Join(artifactsDirectoryName, name+".log")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	env.DeferErr("close log file: "+filename, file.Close)
	return file
}

// TestOPDevnet sets up a standard OP Stack devnet with geth as the execution engine.
// It ensures the node, batcher, and proposer are functioning.
// It only tests the sequencer; no verifiers are configured.
func TestOPDevnet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := environment.New()
	defer func() {
		cancel()
		require.NoError(t, env.Close())
	}()

	if err := os.Mkdir(artifactsDirectoryName, 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}

	log.SetDefault(log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "root"), false)))

	l1Deployments, err := opdevnet.DefaultL1Deployments()
	require.NoError(t, err)
	deployConfig, err := opdevnet.DefaultDeployConfig(l1Deployments)
	require.NoError(t, err)

	l2EngineURL, err := e2eurl.ParseString("ws://127.0.0.1:8890")
	require.NoError(t, err)
	opNodeURL, err := e2eurl.ParseString("http://127.0.0.1:8891")
	require.NoError(t, err)
	l1URL, err := e2eurl.ParseString("ws://127.0.0.1:8892")
	require.NoError(t, err)
	l2EthURL, err := e2eurl.ParseString("ws://127.0.0.1:8893")
	require.NoError(t, err)

	l1Allocs, err := opdevnet.DefaultL1Allocs()
	require.NoError(t, err)
	l1Config, err := opdevnet.BuildL1Config(
		deployConfig,
		l1Deployments,
		l1Allocs,
		l1URL,
		t.TempDir(),
	)
	require.NoError(t, err)
	l1Config.BlobsDirPath = t.TempDir()
	require.NoError(t, l1Config.Run(ctx, env, log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "l1"), false))))
	l1GenesisBlock := l1Config.Genesis.ToBlock()
	require.True(t, l1URL.IsReachable(ctx))

	l2Allocs, err := opdevnet.DefaultL2Allocs()
	require.NoError(t, err)
	l2Genesis, err := genesis.BuildL2Genesis(deployConfig, l2Allocs, l1GenesisBlock)
	require.NoError(t, err)
	jwtSecret := [32]byte{123}

	// Set up and run a sequencer node
	l2Node, l2Backend, err := geth.InitL2("l2", l2Genesis.Config.ChainID, l2Genesis, writeJWT(t, jwtSecret), func(_ *ethconfig.Config, nodeCfg *node.Config) error {
		nodeCfg.AuthAddr = l2EngineURL.Hostname()
		nodeCfg.AuthPort = int(l2EngineURL.PortU16())
		nodeCfg.WSHost = l2EthURL.Hostname()
		nodeCfg.WSPort = int(l2EthURL.PortU16())
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, l2Node.Start())
	defer func() {
		require.NoError(t, l2Node.Close())
	}()
	require.True(t, l2EngineURL.IsReachable(ctx))

	secrets, err := opdevnet.DefaultMnemonicConfig.Secrets()
	require.NoError(t, err)
	opConfig, err := opdevnet.BuildOPConfig(
		deployConfig,
		secrets.Batcher,
		secrets.Proposer,
		l1GenesisBlock,
		l1Deployments.L2OutputOracleProxy,
		eth.HeaderBlockID(l2Genesis.ToBlock().Header()),
		l1Config.URL,
		opNodeURL,
		l2EngineURL,
		l2EthURL,
		jwtSecret,
	)
	require.NoError(t, err)
	require.NoError(t, opConfig.Run(ctx, env, log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "op"), false))))

	l2SlotDuration := time.Duration(opConfig.Node.Rollup.BlockTime) * time.Second

	// Confirm safe blocks are incrementing to ensure the batcher is posting blocks to the DA layer.
	for l2Backend.BlockChain().CurrentSafeBlock().Number.Uint64() < 3 {
		time.Sleep(l2SlotDuration)
	}

	// Ensure the proposer is submitting outputs to L1.
	opNodeRPCClient, err := client.NewRPC(ctx, log.Root(), opNodeURL.String())
	require.NoError(t, err)
	rollupClient := sources.NewRollupClient(opNodeRPCClient)
	for {
		if _, err := rollupClient.OutputAtBlock(ctx, l2Backend.BlockChain().CurrentHeader().Number.Uint64()); err == nil {
			break
		}
		time.Sleep(l2SlotDuration)
	}

	// Set up and run a verifier node
	verifierL2EngineURL, err := e2eurl.ParseString("ws://127.0.0.1:8894")
	require.NoError(t, err)
	verifierOpNodeURL, err := e2eurl.ParseString("http://127.0.0.1:8895")
	require.NoError(t, err)
	verifierL2EthURL, err := e2eurl.ParseString("ws://127.0.0.1:8896")
	require.NoError(t, err)

	verifierL2Node, verifierL2Backend, err := geth.InitL2("l2-verifier", l2Genesis.Config.ChainID, l2Genesis, writeJWT(t, jwtSecret), func(_ *ethconfig.Config, nodeCfg *node.Config) error {
		nodeCfg.AuthAddr = verifierL2EngineURL.Hostname()
		nodeCfg.AuthPort = int(verifierL2EngineURL.PortU16())
		nodeCfg.WSHost = verifierL2EthURL.Hostname()
		nodeCfg.WSPort = int(verifierL2EthURL.PortU16())
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, verifierL2Node.Start())
	defer func() {
		require.NoError(t, verifierL2Node.Close())
	}()
	require.True(t, verifierL2EngineURL.IsReachable(ctx))

	verifierOpConfig, err := opdevnet.BuildOPConfig(
		deployConfig,
		secrets.Batcher,
		secrets.Proposer,
		l1GenesisBlock,
		l1Deployments.L2OutputOracleProxy,
		eth.HeaderBlockID(l2Genesis.ToBlock().Header()),
		l1URL,
		verifierOpNodeURL,
		verifierL2EngineURL,
		verifierL2EthURL,
		jwtSecret,
	)
	require.NoError(t, err)
	verifierOpConfig.Node.Driver.SequencerEnabled = false
	require.NoError(t, verifierOpConfig.Run(ctx, env, log.NewLogger(log.NewTerminalHandler(openLogFile(t, env, "op-verifier"), false))))

	// Wait for the verifier node to sync to block 10
	const waitBlocks = 30
	for i := 0; i < waitBlocks; i++ {
		if verifierL2Backend.BlockChain().CurrentHeader().Number.Uint64() >= 10 {
			// Assert that the verifier and sequencer state roots at block 10 are equal
			require.Equal(t, verifierL2Backend.BlockChain().GetHeaderByNumber(10).Root, l2Backend.BlockChain().GetHeaderByNumber(10).Root)
			break
		} else if i == waitBlocks-1 {
			t.Fatalf("verifier only synced to block %v", verifierL2Backend.BlockChain().CurrentHeader().Number)
		}
		time.Sleep(time.Second * time.Duration(l1Config.BlockTime))
	}
}

// Copied and slightly modified from optimism/op-e2e.
func writeJWT(t testing.TB, jwtSecret [32]byte) string {
	// Sadly the geth node config cannot load JWT secret from memory, it has to be a file
	jwtPath := filepath.Join(t.TempDir(), "jwt_secret")
	require.NoError(t, os.WriteFile(jwtPath, []byte(hexutil.Encode(jwtSecret[:])), 0o600))
	return jwtPath
}
