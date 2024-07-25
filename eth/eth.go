package eth

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
	"github.com/polymerdao/monomer/monomerdb"
)

type ChainID struct {
	chainID *hexutil.Big
	metrics Metrics
}

func NewChainID(chainID *hexutil.Big, metrics Metrics) *ChainID {
	return &ChainID{
		chainID: chainID,
		metrics: metrics,
	}
}

func (e *ChainID) ChainId() *hexutil.Big { //nolint:stylecheck
	defer e.metrics.RecordRPCMethodCall(ChainIDMethodName, time.Now())

	return e.chainID
}

type DB interface {
	BlockByLabel(eth.BlockLabel) (*monomer.Block, error)
	BlockByHeight(uint64) (*monomer.Block, error)
	BlockByHash(common.Hash) (*monomer.Block, error)
}

type Block struct {
	blockStore DB
	chainID    *big.Int
	metrics    Metrics
}

func NewBlock(blockStore DB, chainID *big.Int, metrics Metrics) *Block {
	return &Block{
		blockStore: blockStore,
		chainID:    chainID,
		metrics:    metrics,
	}
}

func (e *Block) GetBlockByNumber(id BlockID, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByNumberMethodName, time.Now())

	block, err := id.Get(e.blockStore)
	if errors.Is(err, monomerdb.ErrNotFound) {
		return nil, ethereum.NotFound
	} else if err != nil {
		return nil, err
	}
	return e.toRPCBlock(block, fullTx)
}

func (e *Block) GetBlockByHash(hash common.Hash, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByHashMethodName, time.Now())

	block, err := e.blockStore.BlockByHash(hash)
	if errors.Is(err, monomerdb.ErrNotFound) {
		return nil, ethereum.NotFound
	} else if err != nil {
		return nil, err
	}
	return e.toRPCBlock(block, fullTx)
}

func (e *Block) toRPCBlock(block *monomer.Block, fullTx bool) (map[string]any, error) {
	ethBlock, err := block.ToEth()
	if err != nil {
		return nil, fmt.Errorf("convert to eth block: %v", err)
	}
	rpcBlock, err := ethapi.SimpleRPCMarshalBlock(ethBlock, fullTx, e.chainID)
	if err != nil {
		return nil, fmt.Errorf("rpc marshal block: %v", err)
	}
	return rpcBlock, nil
}
