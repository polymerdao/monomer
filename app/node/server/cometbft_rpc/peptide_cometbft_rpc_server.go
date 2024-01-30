package peptide

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/armon/go-metrics"
	abciclient "github.com/cometbft/cometbft/abci/client"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/bytes"
	tmlog "github.com/cometbft/cometbft/libs/log"
	cmtpubsub "github.com/cometbft/cometbft/libs/pubsub"
	cmtquery "github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/proxy"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cometRpc "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type Block = eetypes.Block

// CometServer implements the CometBFT RPC server functionality.
type CometServer struct {
	node    Node
	client  abciclient.Client
	address string
	chainId string
	logger  server.Logger
}

// NodeInfo simulates CometBFT node info.
type NodeInfo struct {
	BlockHash   []byte
	AppHash     []byte
	BlockHeight int64
	Time        time.Time
}

// NewNodeInfo returns a new nodeInfo with current time.
func NewNodeInfo(blockHash, appHash []byte, blockHeight int64) NodeInfo {
	return NodeInfo{
		BlockHash:   blockHash,
		AppHash:     appHash,
		BlockHeight: blockHeight,
		Time:        time.Now(),
	}
}

type Node interface {
	OnWebsocketDisconnect(remoteAddr string, logger tmlog.Logger)
	AddToTxMempool(tx bfttypes.Tx)
	LastNodeInfo() NodeInfo
	EarliestNodeInfo() NodeInfo
	ValidatorInfo() ctypes.ValidatorInfo
	EventBus() *bfttypes.EventBus
	GetTxByHash([]byte) (*abcitypes.TxResult, error)
	SearchTx(ctx context.Context, q *cmtquery.Query) ([]*abcitypes.TxResult, error)
	GetBlock(id any) (*Block, error)
	ReportMetrics()
}

// NewCometRpcServer creates a new CometBFT server and a RPC server.
func NewCometRpcServer(
	node Node,
	address string,
	client abciclient.Client,
	chainId string,
	logger server.Logger,
) (*CometServer, *RPCServer) {
	c := &CometServer{node: node, client: client, address: address, chainId: chainId, logger: logger}
	return c, NewRPCServer(address, createRoute(c), node, "peptide-cometbft-rpc-server", logger)
}

func createRoute(c *CometServer) Route {
	return Route{
		//
		// CometBFT rpc API
		// https://docs.cometbft.com/main/rpc/
		//

		// server status
		"echo":   cometRpc.NewRPCFunc(Echo, "msg"),
		"health": cometRpc.NewRPCFunc(Health, ""),
		"status": cometRpc.NewRPCFunc(c.Status, ""),

		// abci API
		"abci_query": cometRpc.NewRPCFunc(c.ABCIQuery, "path,data,height,prove"),
		"abci_info":  cometRpc.NewRPCFunc(c.ABCIInfo, "", cometRpc.Cacheable()),

		// tx broadcast API
		"broadcast_tx_sync":   cometRpc.NewRPCFunc(c.BroadcastTxSync, "tx"),
		"broadcast_tx_async":  cometRpc.NewRPCFunc(c.BroadcastTxAsync, "tx"),
		"broadcast_tx_commit": cometRpc.NewRPCFunc(c.BroadcastTxCommit, "tx"),

		// tx query API
		"tx":        cometRpc.NewRPCFunc(c.Tx, "hash,prove"),
		"tx_search": cometRpc.NewRPCFunc(c.TxSearch, "query,prove,page,per_page,order_by"),

		// events (un)subscription API
		"subscribe":       cometRpc.NewRPCFunc(c.Subscribe, "query"),
		"unsubscribe":     cometRpc.NewRPCFunc(c.Unsubscribe, "query"),
		"unsubscribe_all": cometRpc.NewRPCFunc(c.UnsubscribeAll, ""),

		// block API provides dummy blocks for Cometbft clients (Ignite)
		"block":         cometRpc.NewRPCFunc(c.Block, "height"),
		"block_by_hash": cometRpc.NewRPCFunc(c.BlockByHash, "hash"),
	}
}

func Echo(_ *rpctypes.Context, msg string) (string, error) {
	return msg, nil
}

func Health(*rpctypes.Context) (*ctypes.ResultHealth, error) {
	return &ctypes.ResultHealth{}, nil
}

// More: https://docs.cometbft.com/main/rpc/#/ABCI/abci_query
func (c *CometServer) ABCIQuery(
	_ *rpctypes.Context,
	path string,
	data bytes.HexBytes,
	height int64,
	prove bool,
) (*ctypes.ResultABCIQuery, error) {
	c.logger.Info("Query", "path", path, "data", data.String(), "height", height, "prove", prove)
	telemetry.IncrCounter(1, "query", "ABCIQuery")

	resQuery, err := c.client.QuerySync(abcitypes.RequestQuery{
		Path:   path,
		Data:   data,
		Height: height,
		Prove:  prove,
	})
	if err != nil {
		return nil, err
	}

	return &ctypes.ResultABCIQuery{Response: *resQuery}, nil
}

// More: https://docs.cometbft.com/main/rpc/#/ABCI/abci_info
func (c *CometServer) ABCIInfo(_ *rpctypes.Context) (*ctypes.ResultABCIInfo, error) {
	telemetry.IncrCounter(1, "query", "ABCIInfo")

	resInfo, err := c.client.InfoSync(proxy.RequestInfo)
	if err != nil {
		return nil, err
	}

	return &ctypes.ResultABCIInfo{Response: *resInfo}, nil
}

// Status returns CometBFT status including node info, pubkey, latest block hash, app hash, block height, and block
// time.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/status
func (c *CometServer) Status(_ *rpctypes.Context) (*ctypes.ResultStatus, error) {
	// adding node info so tmClient of CosmJS doesn't complain
	lastNodeInfo := c.node.LastNodeInfo()
	earliestNodeInfo := c.node.EarliestNodeInfo()
	status := &ctypes.ResultStatus{
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID:   "706f6c796d65722d6e6f6465", // "polymer-node"
			ListenAddr:      "localhost:26657",
			Network:         c.chainId,
			Version:         version.TMCoreSemVer,
			Channels:        []byte("0123456789"),
			Moniker:         "polymer",
			ProtocolVersion: p2p.NewProtocolVersion(version.P2PProtocol, version.BlockProtocol, 0),
		},
		SyncInfo: ctypes.SyncInfo{
			LatestBlockHash:   lastNodeInfo.BlockHash,
			LatestAppHash:     lastNodeInfo.AppHash,
			LatestBlockHeight: lastNodeInfo.BlockHeight,
			LatestBlockTime:   lastNodeInfo.Time,

			EarliestBlockHash:   earliestNodeInfo.BlockHash,
			EarliestAppHash:     earliestNodeInfo.AppHash,
			EarliestBlockHeight: earliestNodeInfo.BlockHeight,
			EarliestBlockTime:   earliestNodeInfo.Time,

			CatchingUp: false,
		},
		ValidatorInfo: c.node.ValidatorInfo(),
	}
	return status, nil
}

// BroadcastTxSync returns with the response from CheckTx, but does not wait for DeliverTx (tx execution).
// More: https://docs.cometbft.com/main/rpc/#/Tx/broadcast_tx_sync
func (c *CometServer) BroadcastTxSync(ctx *rpctypes.Context, tx bfttypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	c.logger.Info("BroadcastTxSync", "tx", tx.Hash())
	telemetry.IncrCounter(1, "tx", "BroadcastTxSync")

	request := abcitypes.RequestCheckTx{Tx: tx, Type: abcitypes.CheckTxType_New}
	checkTxResp, err := c.client.CheckTxSync(request)
	if err != nil {
		return nil, err
	}

	c.node.AddToTxMempool(tx)

	return &ctypes.ResultBroadcastTx{
		Code:      checkTxResp.GetCode(),
		Log:       checkTxResp.GetLog(),
		Codespace: checkTxResp.GetCodespace(),
		Hash:      tx.Hash(),
	}, nil
}

// BroadcastTxAsync returns immediately and does not wait for CheckTx or DeliverTx.
// More: https://docs.cometbft.com/main/rpc/#/Tx/broadcast_tx_async
// NOTE: no tx delivery for async mode since we don't have a tx mempool; use BroadcastTxSync or BroadcastTxCommit instead
func (c *CometServer) BroadcastTxAsync(ctx *rpctypes.Context, tx bfttypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	c.logger.Info("BroadcastTxAsync", "tx", tx.Hash())
	telemetry.IncrCounter(1, "tx", "BroadcastTxAsync")

	// since we don't have a tx mempool, we execute tx immediately, same as BroadcastTxSync
	return c.BroadcastTxSync(ctx, tx)
}

// BroadcastTxCommit returns with the response from CheckTx and DeliverTx.
//
// IMPORTANT: use only for testing and development. In production, use BroadcastTxSync or BroadcastTxAsync. You can
// subscribe for the transaction result using JSONRPC via a websocket. See
// https://docs.cometbft.com/main/core/subscription.html
//
// More: https://docs.cometbft.com/main/rpc/#/Tx/broadcast_tx_commit
func (c *CometServer) BroadcastTxCommit(_ *rpctypes.Context, tx bfttypes.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	c.logger.Info("BroadcastTxCommit", "tx", tx.Hash())
	telemetry.IncrCounter(1, "tx", "BroadcastTxCommit")

	request := abcitypes.RequestCheckTx{Tx: tx, Type: abcitypes.CheckTxType_New}
	checkTxResp, err := c.client.CheckTxSync(request)
	if err != nil {
		return nil, err
	}

	// exit early if CheckTx failed
	if checkTxResp.Code != abcitypes.CodeTypeOK {
		return &ctypes.ResultBroadcastTxCommit{
			CheckTx:   *checkTxResp,
			DeliverTx: abcitypes.ResponseDeliverTx{},
			Hash:      tx.Hash(),
		}, nil
	}

	deliverTxResp, err := c.client.DeliverTxSync(abcitypes.RequestDeliverTx{Tx: tx})
	if err != nil {
		log.Fatalf("failed to DeliverTxSync after checkTxSync: %s", err)
	}
	// c.onTxDelivered(tx, deliverTxResp)

	return &ctypes.ResultBroadcastTxCommit{
		CheckTx:   *checkTxResp,
		DeliverTx: *deliverTxResp,
		Hash:      tx.Hash(),
	}, err
}

// // onTxDelivered is called after a tx is successfully delivered, ie. commited to chainApp state.
// // NOTE: this is only called in autoCommitBlock mode, which is incompatible with engine mode.
// func (c *chainServer) onTxDelivered(tx bfttypes.Tx, deliverTxResp *abcitypes.ResponseDeliverTx) error {
// 	info, _ := c.ABCIInfo(nil)
// 	var err error

// 	// TODO: only add batch txs before committing a block to prevent tx index error
// 	// commit block after every tx for now
// 	batch := txindex.NewBatch(1)
// 	txResult := &abcitypes.TxResult{
// 		Height: info.Response.LastBlockHeight,
// 		Index:  0,
// 		Tx:     tx,
// 		Result: *deliverTxResp,
// 	}

// 	if err = batch.Add(txResult); err != nil {
// 		return fmt.Errorf("failed to add tx to Tx Batch: %w", err)
// 	}
// 	if err = c.txIndexer.AddBatch(batch); err != nil {
// 		return fmt.Errorf("failed to add tx to txIndexer: %w", err)
// 	}

// 	// publish tx events
// 	if err := c.node.EventBus().PublishEventTx(bfttypes.EventDataTx{TxResult: *txResult}); err != nil {
// 		c.logger.Error("failed to publish tx event", "err", err)
// 	}
// 	c.logger.Info("onTxDelivered auto-commit", "height", info.Response.LastBlockHeight, "tx", tx.Hash())
// 	return nil
// }

// Subscribe to events via websocket.
func (c *CometServer) Subscribe(ctx *rpctypes.Context, query string) (*ctypes.ResultSubscribe, error) {
	addr := ctx.RemoteAddr()

	c.logger.Info("Subscribe", "remote", addr, "query", query)
	telemetry.IncrCounter(1, "query", "Subscribe")

	parsedQuery, err := cmtquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// SubscribeTimeout is the maximum time we wait to subscribe for an event.
	// must be less than the server's write timeout (see rpcserver.DefaultConfig)
	SubscribeTimeout := 5 * time.Second
	subCtx, cancel := context.WithTimeout(ctx.Context(), SubscribeTimeout)
	defer cancel()

	sub, err := c.node.EventBus().Subscribe(subCtx, addr, parsedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	// settings for ws clients
	subscriptionID := ctx.JSONReq.ID
	var closeIfSlow = true

	// push events to ws client
	go func() {
		for {
			select {

			// push new msg from EventBus
			case msg := <-sub.Out():
				var (
					resultEvent = &ctypes.ResultEvent{Query: query, Data: msg.Data(), Events: msg.Events()}
					resp        = rpctypes.NewRPCSuccessResponse(subscriptionID, resultEvent)
				)
				writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := ctx.WSConn.WriteRPCResponse(writeCtx, resp); err != nil {
					c.logger.Info("Can't write response (slow client)",
						"to", addr, "subscriptionID", subscriptionID, "err", err)

					if closeIfSlow {
						var (
							err  = errors.New("subscription was canceled (reason: slow client)")
							resp = rpctypes.RPCServerError(subscriptionID, err)
						)
						if !ctx.WSConn.TryWriteRPCResponse(resp) {
							c.logger.Info("Can't write response (slow client)",
								"to", addr, "subscriptionID", subscriptionID, "err", err)
						}
						return
					}
				}

			// Cancelled returns a channel that's closed when the subscription is
			// terminated. Normally triggered by the client disconnecting.
			case <-sub.Cancelled():
				if sub.Err() != cmtpubsub.ErrUnsubscribed {
					var reason string
					if sub.Err() == nil {
						reason = "CometBFT exited"
					} else {
						reason = sub.Err().Error()
					}
					var (
						err  = fmt.Errorf("subscription was canceled (reason: %s)", reason)
						resp = rpctypes.RPCServerError(subscriptionID, err)
					)
					if !ctx.WSConn.TryWriteRPCResponse(resp) {
						c.logger.Info("Can't write response (slow client)",
							"to", addr, "subscriptionID", subscriptionID, "err", err)
					}
				}
				return
			}
		}
	}()

	return &ctypes.ResultSubscribe{}, nil
}

// Unsubscribe from events via websocket.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/unsubscribe
func (c *CometServer) Unsubscribe(ctx *rpctypes.Context, query string) (*ctypes.ResultUnsubscribe, error) {
	addr := ctx.RemoteAddr()
	c.logger.Info("Unsubscribe from query", "remote", addr, "query", query)
	telemetry.IncrCounter(1, "query", "Unsubscribe")

	parsedQuery, err := cmtquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	if err := c.node.EventBus().Unsubscribe(ctx.Context(), addr, parsedQuery); err != nil {
		return nil, fmt.Errorf("failed to unsubscribe: %w", err)
	}

	return &ctypes.ResultUnsubscribe{}, nil
}

// UnsubscribeAll unsubscribes from all events via websocket.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/unsubscribe_all
func (c *CometServer) UnsubscribeAll(ctx *rpctypes.Context) (*ctypes.ResultUnsubscribe, error) {
	addr := ctx.RemoteAddr()
	c.logger.Info("Unsubscribe from all events", "remote", addr)
	telemetry.IncrCounter(1, "query", "UnsubscribeAll")

	if err := c.node.EventBus().UnsubscribeAll(ctx.Context(), addr); err != nil {
		return nil, fmt.Errorf("failed to unsubscribe: %w", err)
	}

	return &ctypes.ResultUnsubscribe{}, nil
}

// Tx queries tx and its execution results by tx hash.
// Only mined txs are returned.
// More: https://docs.cometbft.com/main/rpc/#/Tx/tx
//
// NOTE: arg `hash` should be a hex string without 0x prefix
func (c *CometServer) Tx(ctx *rpctypes.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	telemetry.IncrCounterWithLabels([]string{"tx", "Tx"}, 1, []metrics.Label{telemetry.NewLabel("prove",
		strconv.FormatBool(prove))})

	if prove {
		c.logger.Error("prove is not supported")
	}
	r, err := c.node.GetTxByHash(hash)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("tx (%X) not found", hash)
	}
	return &ctypes.ResultTx{
		Hash:     hash,
		Height:   r.Height,
		Index:    r.Index,
		TxResult: r.Result,
		Tx:       r.Tx,
	}, nil
}

// TxSearch queries for multiple tx results. It returns a list of txs (max ?per_page_entries) and total count.
// More: https://docs.cometbft.com/main/rpc/#/Tx/tx_search
//
// param pagePtr: 1-based page number, default (when pagePtr == nil) to 1
// param perPagePtr: number of txs per page, default (when perPagePtr == nil) to 30
// param orderBy: {"", "asc", "desc"}, default (when orderBy == "") to "asc"
func (c *CometServer) TxSearch(
	ctx *rpctypes.Context,
	query string,
	prove bool,
	pagePtr, perPagePtr *int,
	orderBy string,
) (*ctypes.ResultTxSearch, error) {
	if prove {
		c.logger.Error("prove is not supported")
	}
	telemetry.IncrCounter(1, "query", "TxSearch")

	q, err := cmtquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	results, err := c.node.SearchTx(ctx.Context(), q)
	if err != nil {
		return nil, fmt.Errorf("failed to search txs: %w", err)
	}

	// sort results before pagination
	// sort by height, then index
	switch orderBy {
	case "desc":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Height == results[j].Height {
				return results[i].Index > results[j].Index
			} else {
				return results[i].Height > results[j].Height
			}
		})
	// ascending order is the default
	case "asc", "":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Height == results[j].Height {
				return results[i].Index < results[j].Index
			} else {
				return results[i].Height < results[j].Height
			}
		})
	default:
		return nil, fmt.Errorf("expected order_by {asc, desc}, but got: '%s'", orderBy)
	}

	totalCount := len(results)
	paginater := NewPaginater(30) // Set the default page size here
	skipCount, returnedCount, err := paginater.Paginate(pagePtr, perPagePtr, totalCount)
	if err != nil {
		return nil, err
	}

	apiResults := make([]*ctypes.ResultTx, 0, returnedCount)
	for i := skipCount; i < skipCount+returnedCount; i++ {
		r := results[i]
		apiResults = append(apiResults, &ctypes.ResultTx{
			Hash:     bfttypes.Tx(r.Tx).Hash(),
			Height:   r.Height,
			Index:    r.Index,
			TxResult: r.Result,
			Tx:       r.Tx,
		})
	}

	return &ctypes.ResultTxSearch{Txs: apiResults, TotalCount: totalCount}, nil

}

// Block queries block by height.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/block
func (c *CometServer) Block(ctx *rpctypes.Context, height int64) (*ctypes.ResultBlock, error) {
	telemetry.IncrCounter(1, "query", "Block")

	block, err := c.node.GetBlock(height)
	if err != nil {
		return nil, err
	}
	return toCometBlock(block), nil
}

// BlockByHash queries block by hash.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/block_by_hash
func (c *CometServer) BlockByHash(ctx *rpctypes.Context, hash []byte) (*ctypes.ResultBlock, error) {
	telemetry.IncrCounter(1, "query", "BlockByHash")

	block, err := c.node.GetBlock(hash)
	if err != nil {
		return nil, err
	}
	return toCometBlock(block), nil
}

// toCometBlock converts a Peptide block to a CometBFT ResultBlock.
func toCometBlock(block *Block) *ctypes.ResultBlock {
	blockHash := block.Hash()
	return &ctypes.ResultBlock{
		BlockID: bfttypes.BlockID{
			Hash: bytes.HexBytes(blockHash[:]),
		},
		Block: &bfttypes.Block{
			Header: bfttypes.Header{
				ChainID: block.Header.ChainID,
				Time:    time.Unix(int64(block.Header.Time), 0),
				Height:  block.Header.Height,
				AppHash: bytes.HexBytes(block.Header.AppHash),
			},
		},
	}
}

type Paginater struct {
	defaultPageSize int
}

func NewPaginater(defaultPerPage int) *Paginater {
	return &Paginater{
		defaultPageSize: defaultPerPage,
	}
}

// Paginate calculates the skip count and actual page size for pagination based on the given parameters.
// It takes the page number, page size, and total count as inputs and returns the skip count, actual page size, and any error encountered.
// If the page number or page size is invalid, an error is returned.
// The default page size is used if the page size is not provided or is out of range.
// The total number of pages is calculated based on the total count and page size.
// The skip count is calculated as (page - 1) * page size.
// The actual page size is calculated as the minimum of the page size and the remaining count after skipping (boundary check).
func (p *Paginater) Paginate(pagePtr, pageSizePtr *int, totalCount int) (skipCount, pageSize int, err error) {
	// Set the page size
	if pageSizePtr == nil || *pageSizePtr <= 0 || *pageSizePtr > 100 {
		pageSize = p.defaultPageSize
	} else {
		pageSize = *pageSizePtr
	}
	// Calculate the total number of pages
	pages := ((totalCount - 1) / pageSize) + 1

	// Validate and set the page number
	var page int
	if pagePtr == nil {
		page = 1
	} else if *pagePtr <= 0 || *pagePtr > pages {
		err = fmt.Errorf("page must be between 1 and %d, but got %d; totalCount:%d, pageSize: %d", pages, *pagePtr, totalCount, pageSize)
		return
	} else {
		page = *pagePtr
	}

	// Calculate the skip count
	skipCount = (page - 1) * pageSize
	if skipCount < 0 {
		skipCount = 0
	}

	// Calculate the actual page size
	pageSize = minInt(pageSize, totalCount-skipCount)

	return skipCount, pageSize, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
