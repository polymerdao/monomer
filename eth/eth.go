package eth

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/app/peptide/store"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

// var errBlockNotFound = errors.New("block not found") // the op-node checks for this exact string.
type ChainID struct {
	chainID *hexutil.Big
	metrics Metrics
}

// TODO: look into adding a global metrics var instead of passing metrics objects around to constructors
func NewChainID(chainID *hexutil.Big, metrics Metrics) *ChainID {
	return &ChainID{
		chainID: chainID,
		metrics: metrics,
	}
}

func (e *ChainID) ChainId() *hexutil.Big { //nolint:stylecheck
	defer e.metrics.RecordRPCMethodCall(ChainIdMethodName, time.Now())

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

func (e *Block) GetBlockByNumber(id BlockID, inclTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByNumberMethodName, time.Now())

	b := id.Get(e.blockStore)
	if b == nil {
		return nil, ethereum.NotFound
	}
	txs, err := rolluptypes.AdaptCosmosTxsToEthTxs(b.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt cosmos txs: %v", err)
	}
	return b.ToEthLikeBlock(txs, inclTx), nil
}

func (e *Block) GetBlockByHash(hash common.Hash, inclTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByHashMethodName, time.Now())

	block := e.blockStore.BlockByHash(hash)
	if block == nil {
		return nil, ethereum.NotFound
	}
	txs, err := rolluptypes.AdaptCosmosTxsToEthTxs(block.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt cosmos txs: %v", err)
	}
	return block.ToEthLikeBlock(txs, inclTx), nil
}
