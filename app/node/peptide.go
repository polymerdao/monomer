package node

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/armon/go-metrics"
	tmdb "github.com/cometbft/cometbft-db"
	abciclient "github.com/cometbft/cometbft/abci/client"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	tmlog "github.com/cometbft/cometbft/libs/log"
	cmtquery "github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/libs/service"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/app/node/server"
	cometbft_rpc "github.com/polymerdao/monomer/app/node/server/cometbft_rpc"
	"github.com/polymerdao/monomer/app/node/server/engine"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	peptidecommon "github.com/polymerdao/monomer/app/peptide/common"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	rpcee "github.com/polymerdao/monomer/app/peptide/rpc_ee"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
)

const (
	DefaultGasLimit  = 30_000_000
	AppStateDbName   = "appstate"
	BlockStoreDbName = "blockstore"
	TxStoreDbName    = "txstore"
)

type Endpoint = server.Endpoint
type NodeInfo = cometbft_rpc.NodeInfo
type Block = eetypes.Block
type Hash = eetypes.Hash

type AbciClientCreator = func(app abcitypes.Application) abciclient.Client

// NewLocalClient creates a new local client to ABCI server.
func NewLocalClient(app abcitypes.Application) abciclient.Client {
	return abciclient.NewLocalClient(nil, app)
}

// NewPeptideNode creates a new RPC server that
// - compatible with a subset of CometBFT's RPC API that covers txs, queries, and event subscription
// - semantically comptabile with op-geth Execution Engine API
// - supports both json RPC and websocket

func NewPeptideNode(
	bsdb tmdb.DB,
	txstoreDb tmdb.DB,
	appEndpoint *Endpoint,
	eeEndpoint *Endpoint,
	chainApp *peptide.PeptideApp,
	clientCreator AbciClientCreator,
	genesis *PeptideGenesis,
	enabledApis server.ApiEnabledMask,
	logger server.Logger) *PeptideNode {
	bs := store.NewBlockStore(bsdb, eetypes.BlockUnmarshaler)
	txstore := txstore.NewTxStore(txstoreDb, logger)
	node := newNode(chainApp, clientCreator, bs, txstore, genesis, logger)
	cometServer, cometRpcServer := cometbft_rpc.NewCometRpcServer(
		node,
		appEndpoint.FullAddress(),
		node.client,
		chainApp.ChainId,
		logger,
	)

	config := rpcee.DefaultConfig(eeEndpoint.Host)
	eeServer := rpcee.NewEeRpcServer(config, engine.GetExecutionEngineAPIs(node, enabledApis, logger), logger)

	node.cometServer = cometServer
	node.cometRpcServer = cometRpcServer
	node.eeServer = eeServer
	node.nodeServices = server.NewCompositeService(node, cometRpcServer, eeServer)
	return node
}

func NewPeptideNodeFromConfig(
	app *peptide.PeptideApp,
	bsdb tmdb.DB,
	txstoreDb tmdb.DB,
	genesis *PeptideGenesis,
	config *server.Config) (*PeptideNode, error) {
	// TODO: enable abci servers too if configured
	return NewPeptideNode(
		bsdb,
		txstoreDb,
		&config.PeptideCometServerRpc,
		&config.PeptideEngineServerRpc,
		app,
		NewLocalClient,
		genesis,
		config.Apis,
		config.Logger,
	), nil
}

func InitChain(app *peptide.PeptideApp, bsdb tmdb.DB, genesis *PeptideGenesis) (*Block, error) {
	block := Block{}
	l1TxBytes, err := derive.L1InfoDepositBytes(
		0,
		// TODO add l1 parent hash from genesis
		eetypes.NewBlockInfo(genesis.L1.Hash, eetypes.HashOfEmptyHash, genesis.L1.Number, uint64(genesis.GenesisTime.Unix())),
		// TODO fill this out?
		eth.SystemConfig{},
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive L1 info deposit tx bytes for genesis block: %v", err)
	}
	block.L1Txs = []eth.Data{l1TxBytes}

	genesisHeader := app.Init(genesis.AppState, genesis.InitialHeight, genesis.GenesisTime)

	header := eetypes.Header{}
	header.Populate(genesisHeader)
	block.Header = &header
	block.GasLimit = hexutil.Uint64(DefaultGasLimit)

	bs := store.NewBlockStore(bsdb, eetypes.BlockUnmarshaler)

	bs.AddBlock(&block)
	hash := block.Hash()
	for _, label := range []eth.BlockLabel{eth.Unsafe, eth.Finalized, eth.Safe} {
		if err := bs.UpdateLabel(label, hash); err != nil {
			panic(err)
		}
	}

	return &block, nil
}

// PeptideNode implements all RPC methods defined in RouteMap.
type PeptideNode struct {
	genesis       *PeptideGenesis
	client        abciclient.Client
	clientCreator AbciClientCreator
	chainApp      *peptide.PeptideApp
	lock          sync.RWMutex

	eventBus *bfttypes.EventBus
	txstore  txstore.TxStore
	logger   tmlog.Logger

	// simulated node info
	lastNodeInfo     NodeInfo
	earliestNodeInfo NodeInfo

	// TODO: remove this flag since we don't support non-engine mode here
	engineMode bool
	// txResults in current block that's being built. It should be reset after a new block is created for next round.
	txResults []*abcitypes.TxResult

	bs          store.BlockStore
	latestBlock *Block // latest mined block that may not be canonical

	ps payloadstore.PayloadStore
	// L2 txs are stored in mempool until block is sealed
	txMempool bfttypes.Txs

	// Node components
	cometServer    *cometbft_rpc.CometServer
	cometRpcServer *cometbft_rpc.RPCServer
	eeServer       *rpcee.EERPCServer
	*service.BaseService
	nodeServices service.Service
}

// Service returns the composite service that implements
// - node services: p2p, event bus, etc.
// - CometBFT's RPC API
// - Execution Engine API.
//
// Must use this method to start/stop the node.
func (cs *PeptideNode) Service() service.Service {
	return cs.nodeServices
}

// CometServerAddress returns the address of the cometbft rpc server.
func (cs *PeptideNode) CometServerAddress() net.Addr {
	return cs.cometRpcServer.Address()
}

// EngineServerAddress returns the address of the execution engine rpc server.
func (cs *PeptideNode) EngineServerAddress() net.Addr {
	return cs.eeServer.Address()
}

// TODO: do not expose PeptideNode as a service to avoid misuse
var _ service.Service = (*PeptideNode)(nil)

func newNode(chainApp *peptide.PeptideApp, clientCreator AbciClientCreator, bs store.BlockStore,
	txstore txstore.TxStore, genesis *PeptideGenesis, logger tmlog.Logger) *PeptideNode {
	cs := &PeptideNode{
		genesis:       genesis,
		clientCreator: clientCreator,
		chainApp:      chainApp,
		logger:        logger,
		bs:            bs,
		txstore:       txstore,
		engineMode:    true,
		ps:            payloadstore.NewPayloadStore(),
		lock:          sync.RWMutex{},
	}
	cs.BaseService = service.NewBaseService(logger, "PeptideNode", cs)

	cs.resume()
	cs.resetClient()
	return cs
}

func (cs *PeptideNode) resume() {
	lastBlockData := cs.bs.BlockByLabel(eth.Unsafe)
	if lastBlockData == nil {
		panic("could not load current block")
	}

	lastBlock, ok := lastBlockData.(*Block)
	if !ok {
		panic("could not infer current block from BlockData")
	}

	// in the odd case the app state comes up out of sync with the blockstore, we perform a mini-rollback
	// to bring them back to the same place. This should never ever happen but when it does (and it did)
	// it would cause the loop derivation to get stuck
	if lastBlock.Height() != cs.chainApp.LastBlockHeight() {
		cs.logger.Info("blockstore and appstate out of sync",
			"last_block_height",
			lastBlock.Height(),
			"app_last_height",
			cs.chainApp.LastBlockHeight())

		// because the appstate is *always* comitted before the blockstore, the only scenario where there'd be
		// a mismatch is if the appstate is ahead by 1. Other situation would mean something else is broken
		// and there's no point in trying to fix it at runtime.
		if lastBlock.Height()+1 != cs.chainApp.LastBlockHeight() {
			panic("difference between blockstore and appstate is higher than 1")
		}

		// do the mini-rollback to the last height available on the block store
		if err := cs.chainApp.RollbackToHeight(lastBlock.Height()); err != nil {
			panic(err)
		}
	}

	header := &tmproto.Header{
		ChainID:            lastBlock.Header.ChainID,
		Height:             lastBlock.Header.Height,
		Time:               time.UnixMilli(int64(lastBlock.Header.Time)),
		LastCommitHash:     lastBlock.Header.LastCommitHash,
		DataHash:           lastBlock.Header.DataHash,
		ValidatorsHash:     lastBlock.Header.ValidatorsHash,
		NextValidatorsHash: lastBlock.Header.NextValidatorsHash,
		ConsensusHash:      lastBlock.Header.ConsensusHash,
		AppHash:            lastBlock.Header.AppHash,
		LastResultsHash:    lastBlock.Header.LastResultsHash,
		EvidenceHash:       lastBlock.Header.EvidenceHash,
	}

	if err := cs.chainApp.Resume(header, cs.genesis.AppState); err != nil {
		panic(err)
	}
	cs.latestBlock = lastBlock
}

// this performs the rollback to a previous height of all of our stores, removing anything that
// happened *after* the rollback height. If this function suceeds, the chain will resume normal
// operations starting from the block pointed by `head` (i.e. the latest block will
// be that pointed by `head`)
// there's a potential risk with having to rollback different databases, if one of them fails
// the chain' state may be inconsistent. There's no easy way around this so in that case we panic.
func (cs *PeptideNode) Rollback(head, safe, finalized *eetypes.Block) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	currentHeight := cs.chainApp.LastBlockHeight()
	// first, do all the non-mutating checks and error out if something is not right
	if safe.Height() > head.Height() {
		return fmt.Errorf(
			"invalid rollback heights. safe (%v) > unsafe (%v)",
			safe.Height(),
			head.Height(),
		)
	}
	if finalized.Height() > head.Height() {
		return fmt.Errorf(
			"invalid rollback heights. finalized (%v) > unsafe (%v)",
			finalized.Height(),
			head.Height(),
		)
	}
	if head.Height() >= currentHeight {
		return fmt.Errorf(
			"invalid rollback heights. unsafe (%v) >= app.lastblock (%v)",
			head.Height(),
			currentHeight,
		)
	}

	// here's where the actual rollback begins. in case of error, panic away!

	// block store
	if err := cs.bs.RollbackToHeight(head.Height()); err != nil {
		panic(fmt.Sprintf("failed to roll back the block store: %v", err))
	}

	// app
	if err := cs.chainApp.RollbackToHeight(head.Height()); err != nil {
		panic(fmt.Sprintf("failed to roll back the app: %v", err))
	}

	// payload store
	if err := cs.ps.RollbackToHeight(head.Height()); err != nil {
		panic(fmt.Sprintf("failed to roll back the payload store: %v", err))
	}

	// tx indexer / store
	if err := cs.txstore.RollbackToHeight(head.Height(), currentHeight); err != nil {
		panic(fmt.Sprintf("failed to roll back the tx store: %v", err))
	}

	// update the corresponding labels
	cs.bs.UpdateLabel(eth.Finalized, finalized.Hash())
	cs.bs.UpdateLabel(eth.Safe, safe.Hash())
	cs.bs.UpdateLabel(eth.Unsafe, head.Hash())

	cs.logger.Info("Rollback complete",
		"safe_hash", safe.Hash(), "safe_height", safe.Height(),
		"finalized_hash", finalized.Hash(), "finalized_height", finalized.Height(),
		"head_hash", head.Hash(), "head_height", head.Height())

	// resume the app at the very end
	cs.resume()
	return nil
}

// OnStart starts the chain server.
func (cs *PeptideNode) OnStart() error {
	// create and start event bus
	cs.eventBus = bfttypes.NewEventBus()
	cs.eventBus.SetLogger(cs.logger.With("module", "events"))
	if err := cs.eventBus.Start(); err != nil {
		cs.logger.Error("failed to start event bus", "err", err)
		log.Fatalf("failed to start event bus: %s", err)
	}

	cs.RegisterServices()
	return nil
}

// OnStop cleans up resources of clientRoute.
func (cs *PeptideNode) OnStop() {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	if cs.eventBus != nil {
		cs.eventBus.Stop()
	}
	metrics.Shutdown()
}

// OnWebsocketDisconnect is called when a websocket connection is disconnected.
func (cs *PeptideNode) OnWebsocketDisconnect(remoteAddr string, logger tmlog.Logger) {
	logger.Info("OnDisconnect", "remote", remoteAddr)
	err := cs.eventBus.UnsubscribeAll(context.Background(), remoteAddr)
	if err != nil {
		logger.Error("failed to unsubscribe events", "remote", remoteAddr, "err", err)
	}
}

// LastNodeInfo returns the latest node info.
// The nodeInfo is nothing but a placeholder intended only for cometbft rpc server.
func (cs *PeptideNode) LastNodeInfo() NodeInfo {
	return cs.lastNodeInfo
}

// EarliestNodeInfo returns the earliest node info.
// The nodeInfo is nothing but a placeholder intended only for cometbft rpc server.
func (cs *PeptideNode) EarliestNodeInfo() NodeInfo {
	return cs.earliestNodeInfo
}

// AddToTxMempool adds txs to the mempool.
func (cs *PeptideNode) AddToTxMempool(tx bfttypes.Tx) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.txMempool = append(cs.txMempool, tx)
}

type ValidatorInfo = ctypes.ValidatorInfo

func (cs *PeptideNode) ValidatorInfo() ValidatorInfo {
	return ctypes.ValidatorInfo{
		Address:     cs.chainApp.ValSet.Validators[0].Address,
		PubKey:      cs.chainApp.ValSet.Validators[0].PubKey,
		VotingPower: cs.chainApp.ValSet.Validators[0].VotingPower,
	}
}

func (cs *PeptideNode) EventBus() *bfttypes.EventBus {
	return cs.eventBus
}

func (cs *PeptideNode) GetTxByHash(hash []byte) (*abcitypes.TxResult, error) {
	return cs.txstore.Get(hash)
}

func (cs *PeptideNode) GetChainID() string {
	return cs.genesis.ChainID
}

func (cs *PeptideNode) SearchTx(ctx context.Context, q *cmtquery.Query) ([]*abcitypes.TxResult, error) {
	return cs.txstore.Search(ctx, q)
}

func (cs *PeptideNode) getBlockByNumber(number int64) eetypes.BlockData {
	switch ethrpc.BlockNumber(number) {
	// optimism expects these two to be the same
	case ethrpc.PendingBlockNumber, ethrpc.LatestBlockNumber:
		return cs.bs.BlockByLabel(eth.Unsafe)
	case ethrpc.SafeBlockNumber:
		return cs.bs.BlockByLabel(eth.Safe)
	case ethrpc.FinalizedBlockNumber:
		return cs.bs.BlockByLabel(eth.Finalized)
	case ethrpc.EarliestBlockNumber:
		return cs.bs.BlockByHash(cs.genesis.GenesisBlock.Hash)
	default:
		return cs.bs.BlockByNumber(number)
	}
}

func (cs *PeptideNode) getBlockByString(str string) eetypes.BlockData {
	// use base 0 so it's autodetected
	number, err := strconv.ParseInt(str, 0, 64)
	if err == nil {
		return cs.getBlockByNumber(number)
	}
	// When block number is ethrpc.PendingBlockNumber, optimsim expects the latest block.
	// See https://github.com/ethereum-optimism/optimism/blob/v1.2.0/op-e2e/system_test.go#L1353
	// The ethclient converts negative int64 numbers to their respective labels and that's what
	// the server (us) gets. i.e. ethrpc.PendingBlockNumber (-1) is converted to "pending"
	// See https://github.com/ethereum-optimism/op-geth/blob/v1.101304.1/rpc/types.go
	// Since "pending" is no a label we use elsewhere, we need to check for it here
	// and returna the latest (unsafe) block
	if str == "pending" {
		return cs.bs.BlockByLabel(eth.Unsafe)
	}
	return cs.bs.BlockByLabel(eth.BlockLabel(str))
}

func (cs *PeptideNode) GetBlock(id any) (*Block, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	block, err := func() (eetypes.BlockData, error) {
		switch v := id.(type) {
		case nil:
			return cs.bs.BlockByLabel(eth.Unsafe), nil
		case []byte:
			return cs.bs.BlockByHash(Hash(v)), nil
		case int64:
			return cs.getBlockByNumber(v), nil
		// sometimes int values are weirdly converted to float?
		case float64:
			return cs.getBlockByNumber(int64(v)), nil
		case string:
			return cs.getBlockByString(v), nil
		default:
			return nil, fmt.Errorf("cannot query block by value %v (%T)", v, id)
		}
	}()
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, ethereum.NotFound
	}
	return eetypes.MustInferBlock(block), nil
}

func (cs *PeptideNode) UpdateLabel(label eth.BlockLabel, hash Hash) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return cs.bs.UpdateLabel(label, hash)
}

// deliverTxs delivers all txs to the chainApp.
//   - This cause pending chainApp state changes, but does not commit the changes.
//   - App-level tx error will not be bubbled up, but will be included in the tx response for tx events listners and tx quries.
func (cs *PeptideNode) deliverTxs(txs bfttypes.Txs) error {
	height := cs.chainApp.CurrentHeader().Height
	for i, tx := range txs {
		deliverTxResp, err := cs.client.DeliverTxSync(abcitypes.RequestDeliverTx{Tx: tx})
		if err != nil {
			return fmt.Errorf("failed to DeliverTxSync tx-%d in block %d due to: %w", i, height, err)
		}
		txResult := &abcitypes.TxResult{
			Height: height,
			Tx:     tx,
			Result: *deliverTxResp,
		}
		cs.txResults = append(cs.txResults, txResult)
	}
	cs.logger.Info("delivered L2 txs to chainApp", "height", height, "numberOfTxs", len(txs))
	return nil
}

// commitBlockAndUpdateNodeInfo simulates committing current block and updates node info.
func (cs *PeptideNode) commitBlockAndUpdateNodeInfo() {
	block := cs.startBuildingBlock()

	cs.chainApp.CommitAndBeginNextBlock(cs.ps.Current().Attrs.Timestamp)
	block = cs.sealBlock(block)

	cs.bs.AddBlock(block)
	cs.latestBlock = block

	// update last nodeInfo
	cs.lastNodeInfo = cometbft_rpc.NewNodeInfo(
		cs.chainApp.LastCommitID().Hash,
		cs.chainApp.LastHeader().AppHash,
		cs.chainApp.LastBlockHeight(),
	)
}

// startBuildingBlock starts building a new block for App is committed.
func (cs *PeptideNode) startBuildingBlock() *Block {
	// fill in block fields with L1 data
	block := cs.fillBlockWithL1Data(&Block{})
	cs.applyL1Txs(block)
	cs.applyBlockL2Txs(block)
	return block
}

// fillBlockWithL1Data fills in block fields with L1 data.
func (cs *PeptideNode) fillBlockWithL1Data(block *Block) *Block {
	if cs.ps.Current() != nil {
		// must include L1Txs for L2 block's L1Origin
		block.L1Txs = cs.ps.Current().Attrs.Transactions
	} else if cs.engineMode {
		cs.logger.Error("currentPayload is nil for non-genesis block", "blockHeight", block.Height())
		log.Panicf("currentPayload is nil for non-genesis block with height %d", block.Height())
	}
	return block
}

// applyL1Txs applies L1 txs to the block that's currently being built.
func (cs *PeptideNode) applyL1Txs(block *Block) {
	cs.debugL1UserTxs(block.L1Txs, "applyL1Txs.block.L1Txs")

	l1txBytes := make([][]byte, len(block.L1Txs))
	// TODO: better way to marshal L1 tx bytes?
	flattenL1txBytes := make([]byte, 0)
	for i, tx := range block.L1Txs {
		l1txBytes[i] = tx
		flattenL1txBytes = append(flattenL1txBytes, l1txBytes[i]...)
	}

	msg := rolluptypes.NewMsgL1Txs(l1txBytes)
	result, err := cs.chainApp.RunMsgs(cs.chainApp.NewUncachedSdkContext(), msg)
	if err != nil {
		cs.logger.Error("failed to run L1 txs", "err", err)
		log.Panicf("failed to run L1 txs: %s", err)
	}

	// convert sdk.Result to abcitypes.TxResult
	txResult := abcitypes.TxResult{
		Height: cs.chainApp.LastBlockHeight() + 1,
		Tx:     flattenL1txBytes,
		Result: abcitypes.ResponseDeliverTx{
			Log:    result.Log,
			Data:   result.Data,
			Events: result.Events,
		},
	}
	cs.txResults = append(cs.txResults, &txResult)
}

// applyBlockL2Txs applies L2 txs to the block that's currently being built.
func (cs *PeptideNode) applyBlockL2Txs(block *Block) {
	// TODO: ensure txs size doesn't exceed block size limit
	block.Txs = cs.txMempool
	cs.deliverTxs(block.Txs)
	cs.txMempool = nil
}

// indexAndPublishAllTxs indexes all txs in the block that's currently being built.
// This func should only be run once per block; or earlier tx indices will be overwritten by later txs.
func (cs *PeptideNode) indexAndPublishAllTxs(block *Block) error {
	// all L1 txs are flattened to a single txResult returned by chainApp
	if 1+len(block.Txs) != len(cs.txResults) {
		return fmt.Errorf(
			"number of txs (%d) in block %d does not match number of txResults (%d)",
			1+len(block.Txs),
			block.Height(),
			len(cs.txResults),
		)
	}

	if err := cs.txstore.Add(cs.txResults); err != nil {
		return err
	}

	// publish tx events
	for i, txResult := range cs.txResults {
		if err := cs.eventBus.PublishEventTx(bfttypes.EventDataTx{TxResult: *txResult}); err != nil {
			cs.logger.Error("failed to publish tx event", "height", txResult.Height, "index", i, "err", err)
		}
	}
	return nil
}

func (cs *PeptideNode) findParentHash() Hash {
	lastBlock := cs.bs.BlockByNumber(cs.chainApp.LastBlockHeight() - 1)
	if lastBlock != nil {
		return lastBlock.Hash()
	}
	// TODO: handle cases where non-genesis block is missing
	return eetypes.ZeroHash
}

// sealBlock finishes building current L2 block from currentHeader, L2 txs in mempool, and L1 txs from
// payloadAttributes.
//
// sealBlock should be called after chainApp's committed. So chainApp.LastBlockHeight is the sealed block's height
func (cs *PeptideNode) sealBlock(block *Block) *Block {
	cs.logger.Info("seal block", "height", cs.chainApp.LastBlockHeight())

	// finalize block fields
	header := eetypes.Header{}
	block.Header = header.Populate(cs.chainApp.LastHeader())

	payload := cs.ps.Current()
	if payload != nil {
		block.GasLimit = *payload.Attrs.GasLimit
		block.Header.Time = uint64(payload.Attrs.Timestamp)
		block.PrevRandao = payload.Attrs.PrevRandao
	}

	block.ParentBlockHash = cs.findParentHash()

	if err := cs.indexAndPublishAllTxs(block); err != nil {
		cs.logger.Error("failed to index and publish txs", "err", err)
	}
	// reset txResults
	cs.txResults = nil

	return block
}

// CurrentBlock returns the latest canonical block.
// This follows the naming convention of a L2 chain's current canonical block.
func (cs *PeptideNode) CurrentBlock() eetypes.BlockData {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return cs.bs.BlockByLabel(eth.Unsafe)
}

// TODO: add more details to response
type ImportExportResponse struct {
	success bool
	path    string
	height  int64
}

// ExportState exports the current chainApp state to a file.
//
// Set `commit` to true to commit the current block before exporting; otherwise state changes from pending txs are not
// exported.
func (cs *PeptideNode) ExportState(ctx *rpctypes.Context, commit bool, path string) (*ImportExportResponse, error) {
	if commit {
		cs.commitBlockAndUpdateNodeInfo()
	}

	exportedApp := lo.Must(cs.chainApp.ExportAppStateAndValidators(false, nil, nil))
	exportedAppBytes := lo.Must(tmjson.MarshalIndent(exportedApp, "", "  "))

	if err := os.WriteFile(path, exportedAppBytes, os.ModePerm); err != nil {
		return nil, err
	}

	return &ImportExportResponse{success: true, path: path, height: cs.chainApp.LastBlockHeight() + 1}, nil
}

// ImportState imports the exported state from a file.
//
// Call ExportState if you need to persist the current state, since ImportState will reset the chainApp state.
func (cs *PeptideNode) ImportState(ctx *rpctypes.Context, path string, initHeight int64) (*ImportExportResponse, error) {

	exportedBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var exportedApp servertypes.ExportedApp
	tmjson.Unmarshal(exportedBytes, &exportedApp)

	// TODO use the actual state db
	cs.chainApp = peptide.New(cs.chainApp.ChainId, "", tmdb.NewMemDB(), cs.logger)
	cs.resetClient()

	cs.chainApp.ImportAppStateAndValidators(&exportedApp)
	cs.commitBlockAndUpdateNodeInfo()
	return &ImportExportResponse{success: true, path: path, height: cs.chainApp.LastBlockHeight() + 1}, nil
}

// resetClient creates a new client and stops the old one.
func (cs *PeptideNode) resetClient() {
	if cs.client != nil && cs.client.IsRunning() {
		cs.client.Stop()
	}
	cs.client = cs.clientCreator(cs.chainApp)

	// TODO: allow to enable/disable tx indexer; and add logger when cometbft version is upgraded

	// update node info
	cs.earliestNodeInfo = cometbft_rpc.NewNodeInfo(
		cs.latestBlock.Hash().Bytes(),
		cs.chainApp.LastHeader().AppHash,
		cs.chainApp.LastBlockHeight(),
	)
	cs.lastNodeInfo = cs.earliestNodeInfo
}

func (cs *PeptideNode) debugL1UserTxs(txs []hexutil.Bytes, source string) {
	if len(txs) < 2 {
		return
	}
	txBytes := txs[1]
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		cs.logger.Error("failed to unmarshal user deposit transaction", "index", 1, "err", err, "txBytes", txBytes)
	}
	cs.logger.Info("user deposit tx", "source", source, "tx", string(lo.Must(tx.MarshalJSON())), "txBytes", txBytes)
}

// LastBlockHeight returns the last committed block height.
func (cs *PeptideNode) LastBlockHeight() int64 {
	return cs.chainApp.LastBlockHeight()
}

// SavePayload saves the payload by its ID if it's not already in payload cache.
// Also update the latest Payload if this is a new payload
//
// payload must be valid
func (cs *PeptideNode) SavePayload(payload *eetypes.Payload) {
	cs.ps.Add(payload)
}

// GetPayload returns the payload by its ID if it's in payload cache, or (nil, false)
func (cs *PeptideNode) GetPayload(payloadId eetypes.PayloadID) (*eetypes.Payload, bool) {
	return cs.ps.Get(payloadId)
}

// CurrentPayload returns the latest payload.
func (cs *PeptideNode) CurrentPayload() *eetypes.Payload {
	return cs.ps.Current()
}

// CommitBlock commits the current block and starts a new block with incremented block height.
func (cs *PeptideNode) CommitBlock() error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.commitBlockAndUpdateNodeInfo()
	return nil
}

// HeadBlockHash returns the hash of the latest sealed block.
func (cs *PeptideNode) HeadBlockHash() Hash {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return cs.latestBlock.Hash()
}

// GetETH returns the wrapped ETH balance in Wei of the given EVM address.
func (cs *PeptideNode) GetETH(evmAddr common.Address, height int64) (*big.Int, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	cosmAddr := peptidecommon.EvmToCosmos(evmAddr)
	balance, err := cs.chainApp.GetETH(cosmAddr, height)
	if err != nil {
		return nil, err
	}
	return balance.BigInt(), nil
}
