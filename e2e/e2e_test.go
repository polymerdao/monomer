package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/polymerdao/monomer/e2e/url"

	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-node/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/stretchr/testify/require"
)

var e2eTests2 = []struct {
	name string
	run  func(t *testing.T, stack *e2e.StackConfig)
}{
	{
		name: "ETH L1 Deposits and L2 Withdrawals",
		run:  ethRollupFlow,
	},
	/*
		{
			name: "ERC-20 L1 Deposits",
			run:  erc20RollupFlow,
		},

		{
			name: "AttributesTX",
			run:  containsAttributesTx,
		},

		{
			name: "No Rollbacks",
			run:  checkForRollbacks,
			},
	*/
}

func TestE2E2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()
	artifactsDir, err := filepath.Abs("artifacts")
	require.NoError(t, err)

	if err := os.Mkdir(artifactsDir, 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}

	stdoutFile, err := os.OpenFile(filepath.Join(artifactsDir, "stdout"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	require.NoError(t, err)
	env.DeferErr("close stdout file", stdoutFile.Close)
	stdout := os.Stdout
	os.Stdout = stdoutFile
	defer func() {
		os.Stdout = stdout
	}()
	stderrFile, err := os.OpenFile(filepath.Join(artifactsDir, "stderr"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	require.NoError(t, err)
	env.DeferErr("close stderr file", stderrFile.Close)
	stderr := os.Stderr
	os.Stderr = stderrFile
	defer func() {
		os.Stderr = stderr
	}()

	require.NoError(t, e2e.Run(context.Background(), env, t.TempDir()))

	// Run tests concurrently, against the same stack.
	// runningTests := sync.WaitGroup{}
	// runningTests.Add(len(e2eTests))

	for _, test := range e2eTests2 {
		t.Run(test.name, func(t *testing.T) {
			//			go func() {
			//			defer runningTests.Done()
			test.run(t, newStackConfig(t))
			//	}()
		})
	}

	// runningTests.Wait()
}

func newStackConfig(t *testing.T) *e2e.StackConfig {
	secrets, err := opdevnet.DefaultMnemonicConfig.Secrets()
	require.NoError(t, err)

	{
		engineurl, err := url.ParseString("ws://127.0.0.1:9000")
		require.NoError(t, err)
		engineurl.IsReachable(context.Background())
	}
	monomerRPCClient, err := rpc.Dial("ws://127.0.0.1:9000")
	require.NoError(t, err)
	monomerClient := e2e.NewMonomerClient(monomerRPCClient)

	bftClient, err := bftclient.New("tcp://127.0.0.1:26657", "/websocket")
	require.NoError(t, err)
	require.NoError(t, bftClient.Start())

	l1RPCClient, err := rpc.Dial("ws://127.0.0.1:9001")
	require.NoError(t, err)
	l1Client := e2e.NewL1Client(l1RPCClient)

	l1Deployments, err := opdevnet.DefaultL1Deployments()
	require.NoError(t, err)

	opPortal, err := bindings.NewOptimismPortal(l1Deployments.OptimismPortalProxy, l1Client)
	require.NoError(t, err)
	l1StandardBridge, err := opbindings.NewL1StandardBridge(l1Deployments.L1StandardBridgeProxy, l1Client)
	require.NoError(t, err)
	l2OutputOracleCaller, err := bindings.NewL2OutputOracleCaller(l1Deployments.L2OutputOracleProxy, l1Client)
	require.NoError(t, err)

	deployConfig, err := opdevnet.DefaultDeployConfig(l1Deployments)
	require.NoError(t, err)

	return &e2e.StackConfig{
		Ctx:                  context.Background(),
		Users:                []*ecdsa.PrivateKey{secrets.Alice, secrets.Bob},
		L1Client:             l1Client,
		L1Deployments:        l1Deployments,
		OptimismPortal:       opPortal,
		L1StandardBridge:     l1StandardBridge,
		L2OutputOracleCaller: l2OutputOracleCaller,
		L2Client:             bftClient,
		MonomerClient:        monomerClient,
		RollupConfig: &rollup.Config{
			SeqWindowSize: deployConfig.SequencerWindowSize,
		},
		WaitL1: func(numBlocks int) error {
			time.Sleep(time.Second * time.Duration(deployConfig.L1BlockTime*uint64(numBlocks)))
			return nil
		},
		WaitL2: func(numBlocks int) error {
			time.Sleep(time.Second * time.Duration(deployConfig.L2BlockTime*uint64(numBlocks)))
			return nil
		},
	}
}
