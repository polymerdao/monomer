package eth

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
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

type Block struct {
	blockStore store.BlockStoreReader
	metrics    Metrics
}

func NewBlock(blockStore store.BlockStoreReader, metrics Metrics) *Block {
	return &Block{
		blockStore: blockStore,
		metrics:    metrics,
	}
}

func (e *Block) GetBlockByNumber(id BlockID, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByNumberMethodName, time.Now())

	block := id.Get(e.blockStore)
	if block == nil {
		return nil, ethereum.NotFound
	}
	return toRPCBlock(block, fullTx)
}

func (e *Block) GetBlockByHash(hash common.Hash, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByHashMethodName, time.Now())

	block := e.blockStore.BlockByHash(hash)
	if block == nil {
		return nil, ethereum.NotFound
	}
	return toRPCBlock(block, fullTx)
}

func toRPCBlock(block *monomer.Block, fullTx bool) (map[string]any, error) {
	ethBlock, err := block.ToEth()
	if err != nil {
		return nil, fmt.Errorf("convert to eth block: %v", err)
	}
	rpcBlock, err := ethapi.SimpleRPCMarshalBlock(ethBlock, fullTx)
	if err != nil {
		return nil, fmt.Errorf("rpc marshal block: %v", err)
	}
	return rpcBlock, nil
}
