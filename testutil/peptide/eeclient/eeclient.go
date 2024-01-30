package eeclient

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/cometbft/cometbft/libs/bytes"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmrpc "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/retry"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type EeClient struct {
	appClient *tmrpc.Client
	ethClient *ethrpc.Client
	ctx       context.Context
}

func NewEeClient(ctx context.Context, appAddr, eeAddr string) (*EeClient, error) {
	e := &EeClient{ctx: ctx}
	// dialing into the url won't check if the other end is listening so run a quick query on
	// the endpoint to make sure the connection is open.
	// use an exponential-backoff retry mechanism while we are at it
	_, err := retry.Do(ctx, 5, retry.Exponential(), func() (*EeClient, error) {
		var err error
		e.appClient, err = tmrpc.New(appAddr)
		if err != nil {
			return nil, err
		}

		e.ethClient, err = ethrpc.Dial(eeAddr)
		if err != nil {
			return nil, err
		}

		if _, err = e.Health(); err != nil {
			return nil, err
		}
		if _, err = e.ChainID(); err != nil {
			return nil, err
		}
		return e, nil
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (e *EeClient) Close() {
	e.ethClient.Close()
}

// This is taken from the ethclient code. Note that we need to unmarshal the
// response two times to decode the header and the transactions.
func (e *EeClient) blockBy(method string, id any) (*ethtypes.Block, error) {
	var raw json.RawMessage
	if err := e.ethClient.Call(&raw, method, id, true); err != nil {
		return nil, err
	}
	// Decode header and transactions.
	var head *ethtypes.Header
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	// When the block is not found, the API returns JSON null.
	if head == nil {
		return nil, nil
	}
	var body struct {
		Transactions ethtypes.Transactions `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return ethtypes.NewBlockWithHeader(head).WithBody(body.Transactions, nil), nil
}

func (e *EeClient) BlockByNumber(id any) (*ethtypes.Block, error) {
	return e.blockBy("eth_getBlockByNumber", id)
}

func (e *EeClient) BlockByHash(hash eetypes.Hash) (*ethtypes.Block, error) {
	return e.blockBy("eth_getBlockByHash", hash)
}

func (e *EeClient) PeptideBlock(id any) (*eetypes.Block, error) {
	var block eetypes.Block
	if err := e.ethClient.Call(&block, "pep_getBlock", id); err != nil {
		return nil, err
	}
	return &block, nil
}

func (e *EeClient) GetBalance(addr common.Address, number string) (*big.Int, error) {
	var balance hexutil.Big
	if err := e.ethClient.CallContext(e.ctx, &balance, "eth_getBalance", addr, number); err != nil {
		return nil, err
	}
	return (*big.Int)(&balance), nil
}

func (e *EeClient) ChainID() (*big.Int, error) {
	var chainID hexutil.Big
	if err := e.ethClient.Call(&chainID, "eth_chainId"); err != nil {
		return nil, err
	}
	return (*big.Int)(&chainID), nil
}

func (e *EeClient) ForkchoiceUpdated(
	fcs *eth.ForkchoiceState,
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	var fcResult eth.ForkchoiceUpdatedResult
	if err := e.ethClient.Call(&fcResult, "engine_forkchoiceUpdatedV3", fcs, pa); err != nil {
		return nil, err
	}
	return &fcResult, nil
}

func (e *EeClient) NewPayload(payload *eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	var payloadStatus eth.PayloadStatusV1
	if err := e.ethClient.Call(&payloadStatus, "engine_newPayloadV3", payload); err != nil {
		return nil, err
	}
	return &payloadStatus, nil
}

func (e *EeClient) GetPayload(payloadId *eth.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	var payload eth.ExecutionPayloadEnvelope
	if err := e.ethClient.Call(&payload, "engine_getPayloadV3", payloadId); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (e *EeClient) Rollback(latest, safe, finalized int64) error {
	var x json.RawMessage
	return e.ethClient.Call(&x, "admin_rollback", latest, safe, finalized)
}

func (e *EeClient) Tx(hash []byte, prove bool) (*ctypes.ResultTx, error) {
	var result ctypes.ResultTx
	params := map[string]interface{}{
		"hash":  hash,
		"prove": prove,
	}
	if _, err := e.appClient.Call(e.ctx, "tx", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) TxSearch(
	query string,
	prove bool,
	page,
	perPage *int,
	orderBy string,
) (*ctypes.ResultTxSearch, error) {
	var result ctypes.ResultTxSearch
	params := map[string]interface{}{
		"query":    query,
		"prove":    prove,
		"order_by": orderBy,
	}

	if page != nil {
		params["page"] = page
	}
	if perPage != nil {
		params["per_page"] = perPage
	}
	if _, err := e.appClient.Call(e.ctx, "tx_search", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) BroadcastTxSync(txBytes []byte) (*ctypes.ResultBroadcastTx, error) {
	var result ctypes.ResultBroadcastTx
	params := map[string]interface{}{
		"tx": txBytes,
	}
	if _, err := e.appClient.Call(e.ctx, "broadcast_tx_sync", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Block returns block results for given height.
func (e *EeClient) Block(height int64) (*ctypes.ResultBlock, error) {
	var result ctypes.ResultBlock
	params := map[string]interface{}{
		"height": height,
	}
	if _, err := e.appClient.Call(e.ctx, "block", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) CommitBlock() (*ctypes.ResultABCIInfo, error) {
	var result ctypes.ResultABCIInfo
	params := map[string]interface{}{}
	if _, err := e.appClient.Call(e.ctx, "admin_commit_block", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) SetEngineMode(mode bool) error {
	var result ctypes.ResultABCIInfo
	params := map[string]interface{}{
		"mode": mode,
	}
	_, err := e.appClient.Call(e.ctx, "admin_engine_mode", params, &result)
	return err
}

func (e *EeClient) Health() (*ctypes.ResultHealth, error) {
	var result ctypes.ResultHealth
	if _, err := e.appClient.Call(e.ctx, "health", map[string]interface{}{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) ABCIQuery(
	path string,
	data bytes.HexBytes,
	height int64,
	prove bool,
) (*ctypes.ResultABCIQuery, error) {
	var result ctypes.ResultABCIQuery
	params := map[string]interface{}{
		"path":   path,
		"data":   data,
		"height": height,
		"prove":  prove,
	}
	if _, err := e.appClient.Call(e.ctx, "abci_query", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (e *EeClient) Status() (*ctypes.ResultStatus, error) {
	var result ctypes.ResultStatus
	if _, err := e.appClient.Call(e.ctx, "status", map[string]interface{}{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
