package node

import (
	"context"
	"errors"

	"github.com/cometbft/cometbft/libs/bytes"
	client "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	cometbft_rpc "github.com/polymerdao/monomer/app/node/server/cometbft_rpc"
)

// RegisterServices registers all services of the node that may be required by various cosmos chain clients
//
// Prerequisite: the node and its components must be fully initialized
func (n *PeptideNode) RegisterServices() {
	n.registerTxService()
}

// registerTxService registers the tx service for the node
func (n *PeptideNode) registerTxService() {
	tmRpcClient := &tmRpc{server: n.cometServer}
	client := sdkclient.Context{}.WithClient(tmRpcClient).WithTxConfig(n.chainApp.EncodingConfig.TxConfig)
	n.chainApp.RegisterTxService(client)
}

type tmRpc struct {
	server *cometbft_rpc.CometServer
}

var _ sdkclient.TendermintRPC = (*tmRpc)(nil)

// rpcclient.ABCIClient interface implementation

func (t *tmRpc) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	return t.server.ABCIInfo(nil)
}

func (t *tmRpc) ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return t.server.ABCIQuery(nil, path, data, 0, false)
}

func (t *tmRpc) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes,
	opts client.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	return nil, errors.New("not implemented")
}

func (t *tmRpc) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	return t.server.BroadcastTxCommit(nil, tx)
}

func (t *tmRpc) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	return t.server.BroadcastTxAsync(nil, tx)
}

func (t *tmRpc) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	return t.server.BroadcastTxSync(nil, tx)
}

func (t *tmRpc) Validators(ctx context.Context, height *int64, page, perPage *int) (*ctypes.ResultValidators, error) {
	return nil, errors.New("not implemented")
}

func (t *tmRpc) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	return t.server.Status(nil)
}

func (t *tmRpc) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	return t.server.Block(nil, *height)
}

func (t *tmRpc) BlockchainInfo(ctx context.Context, minHeight, maxHeight int64) (*ctypes.ResultBlockchainInfo, error) {
	return nil, errors.New("not implemented")
}

func (t *tmRpc) Commit(ctx context.Context, height *int64) (*ctypes.ResultCommit, error) {
	return nil, errors.New("not implemented")
}

func (t *tmRpc) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	return t.server.Tx(nil, hash, prove)
}

func (t *tmRpc) TxSearch(
	ctx context.Context,
	query string,
	prove bool,
	page, perPage *int,
	orderBy string,
) (*ctypes.ResultTxSearch, error) {
	return t.server.TxSearch(nil, query, prove, page, perPage, orderBy)
}
