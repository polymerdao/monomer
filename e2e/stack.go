package e2e

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/config"
	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/ethereum-optimism/optimism/indexer/bindings"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	nodemetrics "github.com/ethereum-optimism/optimism/op-node/metrics"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	opserviceclient "github.com/ethereum-optimism/optimism/op-service/client"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
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
	Ctx           context.Context
	Operator      L1User
	Users         []L1User
	L1Client      *L1Client
	L1Portal      *bindings.OptimismPortal
	L2Client      *bftclient.HTTP
	MonomerClient *MonomerClient
	stack         *stack
	l1RPCClient   *rpc.Client
	rollupConfig  *rollup.Config
	WaitL1        func(numBlocks int) error
	WaitL2        func(numBlocks int) error
}

type stack struct {
	monomerEngineURL *e2eurl.URL
	monomerCometURL  *e2eurl.URL
	opNodeURL        *e2eurl.URL
	deployConfigDir  string
	l1stateDumpDir   string
	eventListener    EventListener
	prometheusCfg    *config.InstrumentationConfig
}

type L1User struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
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
	deployConfigDir, err := filepath.Abs("./optimism/packages/contracts-bedrock/deploy-config")
	if err != nil {
		return nil, fmt.Errorf("abs deploy config dir: %v", err)
	}
	l1stateDumpDir, err := filepath.Abs("./optimism/.devnet")
	if err != nil {
		return nil, fmt.Errorf("abs l1 state dump dir: %v", err)
	}

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
		deployConfigDir:  deployConfigDir,
		l1stateDumpDir:   l1stateDumpDir,
		eventListener:    eventListener,
		prometheusCfg:    prometheusCfg,
	}

	return stack.run(ctx, env)
}

func (s *stack) run(ctx context.Context, env *environment.Env) (*StackConfig, error) {
	var auxState auxDump

	l1StateJSON, err := os.ReadFile(filepath.Join(s.l1stateDumpDir, "allocs-l1.json"))
	if err != nil {
		// check if not found
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("allocs-l1.json not found - run `make setup-e2e` from project root")
		} else {
			return nil, fmt.Errorf("read allocs-l1.json: %v", err)
		}
	}
	err = json.Unmarshal(l1StateJSON, &auxState)
	if err != nil {
		return nil, fmt.Errorf("unmarshal l1 state: %v", err)
	}

	l1state, err := auxState.ToStateDump()
	if err != nil {
		return nil, fmt.Errorf("auxState to state dump: %v", err)
	}

	// Fund test EOA accounts
	l1users := make([]L1User, 0)
	for range 2 {
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key: %v", err)
		}
		userAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

		user := state.DumpAccount{
			Balance: "0x100000000000000000000000000000000000000000000000000000000000000",
			Nonce:   0,
			Address: &userAddress,
		}
		l1state.OnAccount(user.Address, user)
		l1users = append(l1users, L1User{
			Address:    userAddress,
			PrivateKey: privateKey,
		})
	}

	// configure & run L1

	const networkName = "devnetL1"
	l1Deployments, err := opgenesis.NewL1Deployments(filepath.Join(s.l1stateDumpDir, "addresses.json"))
	if err != nil {
		return nil, fmt.Errorf("new l1 deployments: %v", err)
	}
	deployConfig, err := opgenesis.NewDeployConfigWithNetwork(networkName, s.deployConfigDir)
	if err != nil {
		return nil, fmt.Errorf("new deploy config: %v", err)
	}
	deployConfig.SetDeployments(l1Deployments)
	deployConfig.BatchSenderAddress = l1users[0].Address
	deployConfig.L1GenesisBlockTimestamp = hexutil.Uint64(time.Now().Unix())
	deployConfig.L1UseClique = false // Allows node to produce blocks without addition config. Clique is a PoA config.
	// SequencerWindowSize is usually set to something in the hundreds or thousands.
	// That means we don't ever perform unsafe block consolidation (i.e., the safe head never advances) before the test is complete.
	// To force this edge case to occur in the test, we decrease the SWS.
	deployConfig.SequencerWindowSize = 4
	// Set low ChannelTimeout to ensure the batcher opens and closes a channel in the same block to avoid reorgs in
	// unsafe block consolidation. Note that at the time of this writing, we're still seeing reorgs, so this likely isn't the silver bullet.
	deployConfig.ChannelTimeout = 3
	deployConfig.L1BlockTime = 2
	deployConfig.L2BlockTime = 1

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, l1state, l1Deployments)
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

	for {
		if _, err := l1Client.BlockByNumber(ctx, new(big.Int).SetUint64(1)); err == nil {
			break
		}
	}

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

	opStack := NewOPStack(
		l1url,
		s.monomerEngineURL,
		s.opNodeURL,
		l1Deployments.L2OutputOracleProxy,
		l1users[0].PrivateKey,
		rollupConfig,
		s.eventListener,
	)
	if err := opStack.Run(ctx, env); err != nil {
		return nil, fmt.Errorf("run the op stack: %v", err)
	}

	// construct L2 client
	l2Client, err := bftclient.New(s.monomerCometURL.String(), s.monomerCometURL.String())
	if err != nil {
		return nil, fmt.Errorf("new Comet client: %v", err)
	}

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
		Ctx:           ctx,
		L1Client:      l1Client,
		L1Portal:      opPortal,
		L2Client:      l2Client,
		MonomerClient: monomerClient,
		Operator:      l1users[0],
		Users:         l1users[1:],
		stack:         s,
		rollupConfig:  rollupConfig,
		l1RPCClient:   l1RPCclient,
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

func (s *StackConfig) Derive() error {
	monomerRPCClient, err := rpc.DialContext(s.Ctx, s.stack.monomerEngineURL.String())
	if err != nil {
		return fmt.Errorf("dial monomer: %v", err)
	}
	logger := log.New()
	l2Client, err := sources.NewL2Client(
		opserviceclient.NewBaseRPCClient(monomerRPCClient),
		logger,
		nil,
		sources.L2ClientDefaultConfig(s.rollupConfig, true),
	)
	l1Client, err := sources.NewL1Client(
		opserviceclient.NewBaseRPCClient(s.l1RPCClient),
		logger,
		nil,
		sources.L1ClientDefaultConfig(s.rollupConfig, true, sources.RPCKindBasic),
	)
	if err != nil {
		return fmt.Errorf("new l1 client: %v", err)
	}
	dataSourceFactory := derive.NewDataSourceFactory(logger, s.rollupConfig, l1Client, nil, nil)
	attributesBuilder := derive.NewFetchingAttributesBuilder(s.rollupConfig, l1Client, l2Client)

	l1Traversal := derive.NewL1Traversal(logger, s.rollupConfig, l1Client)
	l1Retrieval := derive.NewL1Retrieval(logger, dataSourceFactory, l1Traversal)
	frameQueue := derive.NewFrameQueue(logger, l1Retrieval)
	channelBank := derive.NewChannelBank(logger, s.rollupConfig, frameQueue, l1Client, nodemetrics.NoopMetrics)
	channelInReader := derive.NewChannelInReader(s.rollupConfig, logger, channelBank, nodemetrics.NoopMetrics)
	bqMiddleware := &batchQueueMiddleware{
		prev: channelInReader,
	}
	batchQueue := derive.NewBatchQueue(logger, s.rollupConfig, bqMiddleware, l2Client)
	attributesQueue := derive.NewAttributesQueue(logger, s.rollupConfig, attributesBuilder, batchQueue)
	eq := &engineQueue{
		attributesQueue: attributesQueue,
		engine:          l2Client,
		rollupConfig:    s.rollupConfig,
		safeHead:        1,
	}
	origin, err := l1Client.L1BlockRefByNumber(context.Background(), 1)
	if err != nil {
		return fmt.Errorf("get genesis block ref by number: %v", err)
	}
	for _, stage := range []derive.ResettableStage{
		eq,
		l1Traversal,
		l1Retrieval,
		frameQueue,
		channelBank,
		channelInReader,
		bqMiddleware,
		batchQueue,
		attributesQueue,
	} {
		if err := stage.Reset(context.Background(), origin, s.rollupConfig.Genesis.SystemConfig); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("reset stage <stage>: %v", err)
		}
	}

	for l1Traversal.Origin().Number <= s.rollupConfig.SeqWindowSize+2 {
		if err := eq.Step(); errors.Is(err, io.EOF) {
			if err = l1Traversal.AdvanceL1Block(context.Background()); err != nil {
				return fmt.Errorf("advance L1 block: %v", err)
			}
		} else if !errors.Is(err, derive.NotEnoughData) {
			return fmt.Errorf("step engine queue: %v", err)
		}
	}
	return nil
}

// we're going to need to mock the engine queue
// perhaps we should mock the attributes queue instead?

type engineQueue struct {
	attributesQueue derive.NextAttributesProvider
	safeHead        uint64
	engine          derive.L2Source
	rollupConfig    *rollup.Config
}

func (eq *engineQueue) Step() error {
	safeHead, err := eq.engine.L2BlockRefByNumber(context.Background(), eq.safeHead)
	if err != nil {
		return fmt.Errorf("l2 block ref by number: %v", err)
	}
	next, err := eq.attributesQueue.NextAttributes(context.Background(), safeHead)
	if errors.Is(err, io.EOF) {
		return io.EOF // sometimes we need err == io.EOF; unfortunately, they don't use errors.Is properly all the time.
	} else if err != nil {
		return fmt.Errorf("next attributes: %v", err)
	}
	attributesBytes, err := json.MarshalIndent(next.Attributes(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal attributes: %v", err)
	}
	fmt.Println(string(attributesBytes))
	eq.safeHead++
	return derive.NotEnoughData
}

func (eq engineQueue) Reset(ctx context.Context, base opeth.L1BlockRef, baseCfg opeth.SystemConfig) error {
	return nil
}

type batchQueueMiddleware struct {
	prev derive.NextBatchProvider
}

var _ derive.NextBatchProvider = (*batchQueueMiddleware)(nil)

func (bq batchQueueMiddleware) Reset(ctx context.Context, base opeth.L1BlockRef, baseCfg opeth.SystemConfig) error {
	return nil
}

func (bq *batchQueueMiddleware) Origin() opeth.L1BlockRef {
	return bq.prev.Origin()
}

func (bq *batchQueueMiddleware) NextBatch(ctx context.Context) (derive.Batch, error) {
	batch, err := bq.prev.NextBatch(ctx)
	if err != nil {
		fmt.Println("heeeeeeeeeeeeeeeeeerrrrrrrreeeeee")
		return nil, err
	}
	singularBatch, ok := batch.(*derive.SingularBatch)
	if !ok {
		panic("not singular batch")
	}
	fmt.Println("batch")
	fmt.Printf("\tepoch num: %d\n", singularBatch.EpochNum)
	fmt.Printf("\ttimestamp: %d\n", singularBatch.Timestamp)
	fmt.Printf("\ttransactions: %s\n", singularBatch.Transactions)
	return batch, nil
}
