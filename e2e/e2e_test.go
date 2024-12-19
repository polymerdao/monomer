package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"testing"
	"time"

	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-node/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/stretchr/testify/require"
)

var e2eTests = []struct {
	name string
	run  func(t *testing.T, stack *e2e.StackConfig)
}{
	{
		name: "ETH L1 Deposits and L2 Withdrawals",
		run:  ethRollupFlow,
	},
	{
		name: "ERC-20 L1 Deposits and L2 Withdrawals",
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
	{
		name: "Verifier Sync",
		run:  verifierSync,
	},
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, e2e.Run(ctx, env, t.TempDir()))

	// Wait for the node setup to finish.
	time.Sleep(time.Second)

	for _, test := range e2eTests {
		t.Run(test.name, func(t *testing.T) {
			test.run(t, newStackConfig(t))
		})
	}
}

func newStackConfig(t *testing.T) *e2e.StackConfig {
	secrets, err := opdevnet.DefaultMnemonicConfig.Secrets()
	require.NoError(t, err)

	engineURL, err := url.ParseString("ws://127.0.0.1:9000")
	require.NoError(t, err)
	require.True(t, engineURL.IsReachable(context.Background()))
	monomerRPCClient, err := rpc.Dial(engineURL.String())
	require.NoError(t, err)
	monomerClient := e2e.NewMonomerClient(monomerRPCClient)

	bftClient, err := bftclient.New("tcp://127.0.0.1:26657", "/websocket")
	require.NoError(t, err)
	require.NoError(t, bftClient.Start())

	l1URL, err := url.ParseString("ws://127.0.0.1:9001")
	require.NoError(t, err)
	require.True(t, l1URL.IsReachable(context.Background()))
	l1RPCClient, err := rpc.Dial(l1URL.String())
	require.NoError(t, err)
	l1Client := e2e.NewL1Client(l1RPCClient)

	verifierEngineURL, err := url.ParseString("ws://127.0.0.1:9010")
	require.NoError(t, err)
	require.True(t, engineURL.IsReachable(context.Background()))
	verifierRPCClient, err := rpc.Dial(verifierEngineURL.String())
	require.NoError(t, err)
	verifierClient := e2e.NewMonomerClient(verifierRPCClient)

	verifierBftClient, err := bftclient.New("tcp://127.0.0.1:26667", "/websocket")
	require.NoError(t, err)
	require.NoError(t, verifierBftClient.Start())

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
		VerifierClient:       verifierClient,
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
