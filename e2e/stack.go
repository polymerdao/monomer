package e2e

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/config"
	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	opbindings "github.com/ethereum-optimism/optimism/op-bindings/bindings"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	ope2econfig "github.com/ethereum-optimism/optimism/op-e2e/config"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum-optimism/optimism/op-node/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/polymerdao/monomer"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
)

type EventListener interface {
	OPEventListener
	node.EventListener
}

type StackConfig struct {
	Ctx                  context.Context
	Users                []*ecdsa.PrivateKey
	L1Client             *L1Client
	L1Deployments        *opgenesis.L1Deployments
	OptimismPortal       *bindings.OptimismPortal
	L1StandardBridge     *opbindings.L1StandardBridge
	L2OutputOracleCaller *bindings.L2OutputOracleCaller
	L2Client             *bftclient.HTTP
	MonomerClient        *MonomerClient
	RollupConfig         *rollup.Config
	WaitL1               func(numBlocks int) error
	WaitL2               func(numBlocks int) error
}

type stack struct {
	monomerEngineURL *e2eurl.URL
	monomerCometURL  *e2eurl.URL
	opNodeURL        *e2eurl.URL
	eventListener    EventListener
	prometheusCfg    *config.InstrumentationConfig
}

// Setup creates and runs a new stack for end-to-end testing.
//
// It assumes availability of hard-coded local URLs for the Monomer engine, Comet, and OP node.
//
// It returns a StackConfig with L1 and L2 clients, the rollup config, and operator and user accounts.
func Setup(
	ctx context.Context,
	env *environment.Env,
	prometheusCfg *config.InstrumentationConfig,
	eventListener EventListener,
) (*StackConfig, error) {
	monomerEngineURL, err := e2eurl.ParseString("ws://127.0.0.1:8889")
	if err != nil {
		return nil, fmt.Errorf("new monomer url: %v", err)
	}
	monomerCometURL, err := e2eurl.ParseString("http://127.0.0.1:8890")
	if err != nil {
		return nil, fmt.Errorf("new cometBFT url: %v", err)
	}
	opNodeURL, err := e2eurl.ParseString("http://127.0.0.1:8891")
	if err != nil {
		return nil, fmt.Errorf("new op-node url: %v", err)
	}

	stack := stack{
		monomerEngineURL: monomerEngineURL,
		monomerCometURL:  monomerCometURL,
		opNodeURL:        opNodeURL,
		eventListener:    eventListener,
		prometheusCfg:    prometheusCfg,
	}

	return stack.run(ctx, env)
}

func (s *stack) run(ctx context.Context, env *environment.Env) (*StackConfig, error) {
	// configure & run L1

	deployConfig := ope2econfig.DeployConfig.Copy()
	// Set a shorter Sequencer Window Size to force unsafe block consolidation to happen more often.
	// A verifier (and the sequencer when it's determining the safe head) will have to read the entire sequencer window
	// before advancing in the worst case. For the sake of tests running quickly, we minimize that worst case to 4 blocks.
	deployConfig.SequencerWindowSize = 4

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, ope2econfig.L1Allocs, ope2econfig.L1Deployments)
	if err != nil {
		return nil, fmt.Errorf("build l1 developer genesis: %v", err)
	}

	l1RPCclient, l1HTTPendpoint, err := gethdevnet(env, deployConfig.L1BlockTime, l1genesis)
	if err != nil {
		return nil, fmt.Errorf("ethdevnet: %v", err)
	}

	l1url, err := e2eurl.ParseString(l1HTTPendpoint)
	if err != nil {
		return nil, fmt.Errorf("new l1 url: %v", err)
	}

	// NOTE: should we set a timeout on the context? Might not be worth the complexity.
	if !l1url.IsReachable(ctx) {
		return nil, fmt.Errorf("l1 url not reachable: %s", l1url.String())
	}

	l1Client := NewL1Client(l1RPCclient)

	latestL1Block, err := l1Client.BlockByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get the latest l1 block: %v", err)
	}

	// Run Monomer.
	if err := s.runMonomer(ctx, env, latestL1Block.Time(), deployConfig.L2ChainID); err != nil {
		return nil, err
	}
	if !s.monomerEngineURL.IsReachable(ctx) {
		return nil, fmt.Errorf("reaching monomer url: %s", s.monomerEngineURL.String())
	}
	monomerRPCClient, err := rpc.DialContext(ctx, s.monomerEngineURL.String())
	if err != nil {
		return nil, fmt.Errorf("dial monomer: %v", err)
	}
	monomerClient := NewMonomerClient(monomerRPCClient)
	l2GenesisBlockHash, err := monomerClient.GenesisHash(ctx)
	if err != nil {
		return nil, fmt.Errorf("get Monomer genesis block hash: %v", err)
	}

	rollupConfig, err := deployConfig.RollupConfig(latestL1Block, l2GenesisBlockHash, 1)
	if err != nil {
		return nil, fmt.Errorf("new rollup config: %v", err)
	}

	opPortal, err := bindings.NewOptimismPortal(rollupConfig.DepositContractAddress, l1Client)
	if err != nil {
		return nil, fmt.Errorf("new optimism portal: %v", err)
	}

	l1StandardBridge, err := opbindings.NewL1StandardBridge(ope2econfig.L1Deployments.L1StandardBridgeProxy, l1Client)
	if err != nil {
		return nil, fmt.Errorf("new l1 standard bridge: %v", err)
	}

	l2OutputOracleCaller, err := bindings.NewL2OutputOracleCaller(ope2econfig.L1Deployments.L2OutputOracleProxy, l1Client)
	if err != nil {
		return nil, fmt.Errorf("new l2 output oracle caller: %v", err)
	}

	secrets, err := e2eutils.DefaultMnemonicConfig.Secrets()
	if err != nil {
		return nil, fmt.Errorf("get secrets for default mnemonics: %v", err)
	}

	opStack := NewOPStack(
		l1url,
		s.monomerEngineURL,
		s.opNodeURL,
		ope2econfig.L1Deployments.L2OutputOracleProxy,
		secrets.Batcher,
		secrets.Proposer,
		rollupConfig,
		s.eventListener,
	)
	if err := opStack.Run(ctx, env); err != nil {
		return nil, fmt.Errorf("run the op stack: %v", err)
	}

	// construct L2 client
	l2Client, err := bftclient.New(s.monomerCometURL.String(), "/websocket")
	if err != nil {
		return nil, fmt.Errorf("new Comet client: %v", err)
	}

	// start the L2 client
	if err = l2Client.Start(); err != nil {
		return nil, fmt.Errorf("start Comet client: %v", err)
	}
	env.DeferErr("stop Comet client", l2Client.Stop)

	wait := func(numBlocks, layer int) error {
		var client interface {
			BlockByNumber(context.Context, *big.Int) (*ethtypes.Block, error)
		}
		if layer == 1 {
			client = l1Client
		} else {
			client = monomerClient
		}

		currentBlock, err := client.BlockByNumber(ctx, nil)
		if err != nil {
			return fmt.Errorf("get the current L%d block: %v", layer, err)
		}
		for {
			latestBlock, err := client.BlockByNumber(ctx, nil)
			if err != nil {
				return fmt.Errorf("get the latest L%d block: %v", layer, err)
			}
			if latestBlock.Number().Int64() >= currentBlock.Number().Int64()+int64(numBlocks) {
				break
			}
			time.Sleep(250 * time.Millisecond) //nolint:mnd
		}
		return nil
	}

	return &StackConfig{
		Ctx:                  ctx,
		L1Client:             l1Client,
		L1Deployments:        ope2econfig.L1Deployments,
		OptimismPortal:       opPortal,
		L1StandardBridge:     l1StandardBridge,
		L2OutputOracleCaller: l2OutputOracleCaller,
		L2Client:             l2Client,
		MonomerClient:        monomerClient,
		Users:                []*ecdsa.PrivateKey{secrets.Alice, secrets.Bob},
		RollupConfig:         rollupConfig,
		WaitL1: func(numBlocks int) error {
			return wait(numBlocks, 1)
		},
		WaitL2: func(numBlocks int) error {
			return wait(numBlocks, 2)
		},
	}, nil
}

func (s *stack) runMonomer(ctx context.Context, env *environment.Env, genesisTime, chainIDU64 uint64) error {
	engineWS, err := net.Listen("tcp", s.monomerEngineURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer engine ws listener: %v", err)
	}
	cometListener, err := net.Listen("tcp", s.monomerCometURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer comet listener: %v", err)
	}
	chainID := monomer.ChainID(chainIDU64)
	app, err := testapp.New(dbm.NewMemDB(), chainID.String())
	if err != nil {
		return fmt.Errorf("new test app: %v", err)
	}

	sdkclient, err := client.NewClientFromNode(s.monomerCometURL.String())
	if err != nil {
		return fmt.Errorf("new client from node: %v", err)
	}
	appchainCtx := client.Context{}.
		WithChainID(chainID.String()).
		WithClient(sdkclient).
		WithAccountRetriever(mockAccountRetriever{}).
		WithTxConfig(testutil.MakeTestTxConfig()).
		WithCodec(testutil.MakeTestEncodingConfig().Codec)

	blockPebbleDB, err := pebble.Open("", &pebble.Options{
		FS: vfs.NewMem(),
	})
	if err != nil {
		return fmt.Errorf("open block db: %v", err)
	}
	env.DeferErr("close block db", blockPebbleDB.Close)
	txdb := cometdb.NewMemDB()
	env.DeferErr("close tx db", txdb.Close)
	mempooldb := dbm.NewMemDB()
	env.DeferErr("close mempool db", mempooldb.Close)
	rawDB := rawdb.NewMemoryDatabase()
	env.DeferErr("close raw db", rawDB.Close)
	trieDB := triedb.NewDatabase(rawDB, nil)
	env.DeferErr("close trieDB", trieDB.Close)
	ethstatedb := state.NewDatabaseWithNodeDB(rawDB, trieDB)
	n := node.New(
		app,
		&appchainCtx,
		&genesis.Genesis{
			AppState: app.DefaultGenesis(),
			ChainID:  chainID,
			Time:     genesisTime,
		},
		engineWS,
		cometListener,
		localdb.New(blockPebbleDB),
		mempooldb,
		txdb,
		ethstatedb,
		s.prometheusCfg,
		s.eventListener,
	)
	if err := n.Run(ctx, env); err != nil {
		return fmt.Errorf("run monomer: %v", err)
	}
	return nil
}
