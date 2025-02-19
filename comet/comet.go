package comet

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bftbytes "github.com/cometbft/cometbft/libs/bytes"
	bftpubsub "github.com/cometbft/cometbft/libs/pubsub"
	bftquery "github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/p2p"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	jsonrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer"
	"github.com/sourcegraph/conc"
)

var errProvingNotSupported = errors.New("proving is not supported")

type AppABCI interface {
	Info(context.Context, *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error)
	Query(context.Context, *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error)
}

type ABCI struct {
	app AppABCI
}

func NewABCI(app AppABCI) *ABCI {
	return &ABCI{
		app: app,
	}
}

func (s *ABCI) Query(
	ctx *jsonrpctypes.Context,
	path string,
	data bftbytes.HexBytes,
	height int64,
	prove bool,
) (*rpctypes.ResultABCIQuery, error) {
	resp, err := s.app.Query(ctx.Context(), &abcitypes.RequestQuery{
		Path:   path,
		Data:   data,
		Height: height,
		Prove:  prove,
	})
	if err != nil {
		return nil, fmt.Errorf("query: %v", err)
	}
	return &rpctypes.ResultABCIQuery{
		Response: *resp,
	}, nil
}

func (s *ABCI) Info(ctx *jsonrpctypes.Context) (*rpctypes.ResultABCIInfo, error) {
	resp, err := s.app.Info(ctx.Context(), &abcitypes.RequestInfo{})
	if err != nil {
		return nil, fmt.Errorf("info: %v", err)
	}
	return &rpctypes.ResultABCIInfo{
		Response: *resp,
	}, nil
}

type HeadHeader interface {
	HeadHeader() (*monomer.Header, error)
}

type StatusAPI struct {
	blockstore HeadHeader
	startBlock *bfttypes.Block
}

func NewStatusAPI(blockStore HeadHeader, startBlock *bfttypes.Block) *StatusAPI {
	return &StatusAPI{
		blockstore: blockStore,
		startBlock: startBlock,
	}
}

// Status returns CometBFT status including node info, pubkey, latest block hash, app hash, block height, and block
// time.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/status
func (s *StatusAPI) Status(_ *jsonrpctypes.Context) (*rpctypes.ResultStatus, error) {
	headHeader, err := s.blockstore.HeadHeader()
	if err != nil {
		return nil, err
	}
	headCometHeader := headHeader.ToComet()
	status := &rpctypes.ResultStatus{
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID:   "",
			ListenAddr:      "",
			Network:         headCometHeader.ChainID,
			Version:         version.TMCoreSemVer,
			Channels:        []byte("0123456789"),
			Moniker:         "monomer",
			ProtocolVersion: p2p.NewProtocolVersion(version.P2PProtocol, version.BlockProtocol, 0),
		},
		// We need SyncInfo so the CosmJS tmClient doesn't complain.
		SyncInfo: rpctypes.SyncInfo{
			LatestBlockHash:   headCometHeader.Hash(),
			LatestAppHash:     headCometHeader.AppHash,
			LatestBlockHeight: headCometHeader.Height,
			LatestBlockTime:   headCometHeader.Time,

			EarliestBlockHash:   s.startBlock.Hash(),
			EarliestAppHash:     s.startBlock.AppHash,
			EarliestBlockHeight: s.startBlock.Height,
			EarliestBlockTime:   s.startBlock.Time,

			CatchingUp: false,
		},
		ValidatorInfo: rpctypes.ValidatorInfo{},
	}
	return status, nil
}

type AppMempool interface {
	CheckTx(context.Context, *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error)
}

type Mempool interface {
	Enqueue(userTxn bfttypes.Tx) error
}

type BroadcastTxAPI struct {
	app     AppMempool
	mempool Mempool
}

func NewBroadcastTxAPI(app AppMempool, mempool Mempool) *BroadcastTxAPI {
	return &BroadcastTxAPI{
		app:     app,
		mempool: mempool,
	}
}

// BroadcastTx returns with the response from CheckTx, but does not wait for DeliverTx (tx execution).
// More: https://docs.cometbft.com/main/rpc/#/Tx/broadcast_tx_sync
func (s *BroadcastTxAPI) BroadcastTx(ctx *jsonrpctypes.Context, tx bfttypes.Tx) (*rpctypes.ResultBroadcastTx, error) {
	checkTxResp, err := s.app.CheckTx(ctx.Context(), &abcitypes.RequestCheckTx{
		Tx:   tx,
		Type: abcitypes.CheckTxType_New,
	})
	if err != nil {
		return nil, fmt.Errorf("check tx: %v", err)
	}
	if checkTxResp.IsOK() {
		if err := s.mempool.Enqueue(tx); err != nil {
			return nil, fmt.Errorf("enqueue in mempool: %v", err)
		}
	}
	return &rpctypes.ResultBroadcastTx{
		Code:      checkTxResp.GetCode(),
		Log:       checkTxResp.GetLog(),
		Codespace: checkTxResp.GetCodespace(),
		Hash:      tx.Hash(),
	}, nil
}

type EventBus interface {
	Subscribe(ctx context.Context, subscriber string, query bftpubsub.Query, outCapacity ...int) (bfttypes.Subscription, error)
	Unsubscribe(ctx context.Context, subscriber string, query bftpubsub.Query) error
	UnsubscribeAll(ctx context.Context, subscriber string) error
}

type SubscribeEventListener interface {
	// err will never be nil.
	OnSubscriptionWriteErr(err error)
	// err may be nil.
	OnSubscriptionCanceled(err error)
}

type SubscriberAPI struct {
	eventBus      EventBus
	wg            *conc.WaitGroup
	eventListener SubscribeEventListener
}

func NewSubscriberAPI(eventBus EventBus, wg *conc.WaitGroup, eventListener SubscribeEventListener) *SubscriberAPI {
	return &SubscriberAPI{
		eventBus:      eventBus,
		wg:            wg,
		eventListener: eventListener,
	}
}

// Subscribe to events via websocket.
func (s *SubscriberAPI) Subscribe(ctx *jsonrpctypes.Context, query string) (*rpctypes.ResultSubscribe, error) {
	parsedQuery, err := bftquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("parse query: %w", err)
	}

	// From CometBFT:
	//   The timeout is the maximum time we wait to subscribe for an event.
	//   Must be less than the http server's write timeout.
	subCtx, cancel := context.WithTimeout(ctx.Context(), 5*time.Second) //nolint:mnd
	defer cancel()

	sub, err := s.eventBus.Subscribe(subCtx, ctx.RemoteAddr(), parsedQuery)
	if err != nil {
		return nil, fmt.Errorf("subscribe to event bus: %w", err)
	}

	subscriptionID := ctx.JSONReq.ID

	// push events to ws client
	s.wg.Go(func() {
		for {
			select {
			case msg := <-sub.Out():
				resultEvent := &rpctypes.ResultEvent{
					Query:  query,
					Data:   msg.Data(),
					Events: msg.Events(),
				}
				resp := jsonrpctypes.NewRPCSuccessResponse(subscriptionID, resultEvent)
				writeCtx, cancel := context.WithTimeout(ctx.Context(), 10*time.Second) //nolint:mnd
				if writeErr := ctx.WSConn.WriteRPCResponse(writeCtx, resp); writeErr != nil {
					cancel()
					err := fmt.Errorf("subscription was canceled (reason: %v)", writeErr)
					resp = jsonrpctypes.RPCServerError(subscriptionID, err)
					ctx.WSConn.TryWriteRPCResponse(resp)
					s.eventListener.OnSubscriptionWriteErr(err)
					return
				}
				cancel()
			case <-sub.Canceled():
				err = sub.Err()
				if errors.Is(err, bftpubsub.ErrUnsubscribed) {
					err = nil
				} else {
					var reason string
					if err == nil {
						reason = "Monomer exited"
					} else {
						reason = err.Error()
					}
					err = fmt.Errorf("subscription was canceled (reason: %s)", reason)
					resp := jsonrpctypes.RPCServerError(subscriptionID, err)
					ctx.WSConn.TryWriteRPCResponse(resp)
				}
				s.eventListener.OnSubscriptionCanceled(err)
				return
			}
		}
	})

	return &rpctypes.ResultSubscribe{}, nil
}

// Unsubscribe from events via websocket.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/unsubscribe
func (s *SubscriberAPI) Unsubscribe(ctx *jsonrpctypes.Context, query string) (*rpctypes.ResultUnsubscribe, error) {
	parsedQuery, err := bftquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("parse query: %w", err)
	}
	if err := s.eventBus.Unsubscribe(ctx.Context(), ctx.RemoteAddr(), parsedQuery); err != nil {
		return nil, fmt.Errorf("unsubscribe from event bus: %w", err)
	}
	return &rpctypes.ResultUnsubscribe{}, nil
}

// UnsubscribeAll unsubscribes from all events via websocket.
// More: https://docs.cometbft.com/main/rpc/#/ABCI/unsubscribe_all
func (s *SubscriberAPI) UnsubscribeAll(ctx *jsonrpctypes.Context) (*rpctypes.ResultUnsubscribe, error) {
	if err := s.eventBus.UnsubscribeAll(ctx.Context(), ctx.RemoteAddr()); err != nil {
		return nil, fmt.Errorf("unsubscribe all: %v", err)
	}
	return &rpctypes.ResultUnsubscribe{}, nil
}

type TxStore interface {
	Get(hash []byte) (*abcitypes.TxResult, error)
	Search(ctx context.Context, q *bftquery.Query) ([]*abcitypes.TxResult, error)
}

type TxAPI struct {
	txstore TxStore
}

func NewTxAPI(txStore TxStore) *TxAPI {
	return &TxAPI{
		txstore: txStore,
	}
}

// https://docs.cometbft.com/main/rpc/#/Tx/tx
// NOTE: arg `hash` should be a hex string without 0x prefix
func (s *TxAPI) ByHash(_ *jsonrpctypes.Context, hash []byte, prove bool) (*rpctypes.ResultTx, error) {
	if prove {
		return nil, errProvingNotSupported
	}

	r, err := s.txstore.Get(hash)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("tx not found: %x", hash)
	}
	return &rpctypes.ResultTx{
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
func (s *TxAPI) Search(
	ctx *jsonrpctypes.Context,
	query string,
	prove bool,
	pagePtr,
	perPagePtr *int,
	orderBy string,
) (*rpctypes.ResultTxSearch, error) {
	if prove {
		return nil, errProvingNotSupported
	}

	q, err := bftquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	results, err := s.txstore.Search(ctx.Context(), q)
	if err != nil {
		return nil, fmt.Errorf("failed to search txs: %w", err)
	}

	// Sort results before pagination. Sort by height, then index.
	switch orderBy {
	case "desc":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Height == results[j].Height {
				return results[i].Index > results[j].Index
			} else {
				return results[i].Height > results[j].Height
			}
		})
	case "asc", "": // Ascending order is the default.
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
	skipCount, returnedCount, err := paginate(pagePtr, perPagePtr, totalCount)
	if err != nil {
		return nil, err
	}

	apiResults := make([]*rpctypes.ResultTx, 0, returnedCount)
	for i := skipCount; i < skipCount+returnedCount; i++ {
		r := results[i]
		apiResults = append(apiResults, &rpctypes.ResultTx{
			Hash:     bfttypes.Tx(r.Tx).Hash(),
			Height:   r.Height,
			Index:    r.Index,
			TxResult: r.Result,
			Tx:       r.Tx,
		})
	}

	return &rpctypes.ResultTxSearch{Txs: apiResults, TotalCount: totalCount}, nil
}

// Paginate calculates the skip count and actual page size for pagination based on the given parameters.
// It takes the page number, page size, and total count as inputs and returns the skip count, actual page size, and any error encountered.
// If the page number or page size is invalid, an error is returned.
// The default page size is used if the page size is not provided or is out of range.
// The total number of pages is calculated based on the total count and page size.
// The skip count is calculated as (page - 1) * page size.
// The actual page size is calculated as the minimum of the page size and the remaining count after skipping (boundary check).
func paginate(pagePtr, pageSizePtr *int, totalCount int) (int, int, error) {
	var pageSize int
	if pageSizePtr == nil || *pageSizePtr <= 0 || *pageSizePtr > 100 {
		pageSize = 30
	} else {
		pageSize = *pageSizePtr
	}

	numPages := ((totalCount - 1) / pageSize) + 1

	var pageNum int
	if pagePtr == nil {
		pageNum = 1
	} else {
		pageNum = *pagePtr
	}

	if pageNum <= 0 || pageNum > numPages {
		return 0, 0, fmt.Errorf(
			"page must be between 1 and %d, but got %d; totalCount: %d, pageSize: %d",
			numPages,
			pageNum,
			totalCount,
			pageSize,
		)
	}

	// Calculate the skip count.
	skipCount := (pageNum - 1) * pageSize
	if skipCount < 0 {
		skipCount = 0
	}

	// Calculate the actual page size
	if count := totalCount - skipCount; count < pageSize {
		pageSize = count
	}

	return skipCount, pageSize, nil
}

type DB interface {
	BlockByHash(common.Hash) (*monomer.Block, error)
	BlockByHeight(uint64) (*monomer.Block, error)
}

type BlockAPI struct {
	blockstore DB
}

func NewBlockAPI(blockStore DB) *BlockAPI {
	return &BlockAPI{
		blockstore: blockStore,
	}
}

// https://docs.cometbft.com/main/rpc/#/ABCI/block
func (s *BlockAPI) ByHeight(_ *jsonrpctypes.Context, height int64) (*rpctypes.ResultBlock, error) {
	block, err := s.blockstore.BlockByHeight(uint64(height))
	if err != nil {
		return nil, err
	}
	return rpcBlock(block.ToCometLikeBlock()), nil
}

// https://docs.cometbft.com/main/rpc/#/ABCI/block_by_hash
func (s *BlockAPI) ByHash(_ *jsonrpctypes.Context, hash []byte) (*rpctypes.ResultBlock, error) {
	block, err := s.blockstore.BlockByHash(common.BytesToHash(hash))
	if err != nil {
		return nil, err
	}
	return rpcBlock(block.ToCometLikeBlock()), nil
}

func rpcBlock(block *bfttypes.Block) *rpctypes.ResultBlock {
	return &rpctypes.ResultBlock{
		BlockID: bfttypes.BlockID{
			Hash: block.Hash(),
		},
		Block: block,
	}
}
