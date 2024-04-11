package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
)

// var errBlockNotFound = errors.New("block not found") // the op-node checks for this exact string.
type ChainID struct {
	chainID *hexutil.Big
}

func NewChainID(chainID *hexutil.Big) *ChainID {
	return &ChainID{
		chainID: chainID,
	}
}

func (e *ChainID) ChainId() *hexutil.Big { //nolint:stylecheck
	return e.chainID
}

type BlockByNumber struct {
	blockStore store.BlockStoreReader
	adapter    monomer.CosmosTxAdapter
}

func NewBlockByNumber(blockStore store.BlockStoreReader, adapter monomer.CosmosTxAdapter) *BlockByNumber {
	return &BlockByNumber{
		blockStore: blockStore,
		adapter:    adapter,
	}
}

func (e *BlockByNumber) GetBlockByNumber(id BlockID, inclTx bool) (map[string]any, error) {
	b := id.Get(e.blockStore)
	if b == nil {
		return nil, ethereum.NotFound
	}
	txs, err := e.adapter(b.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt cosmos txs: %v", err)
	}
	return b.ToEthLikeBlock(txs, inclTx), nil
}

type BlockByHash struct {
	blockStore store.BlockStoreReader
	adapter    monomer.CosmosTxAdapter
}

func NewBlockByHash(blockStore store.BlockStoreReader, adapter monomer.CosmosTxAdapter) *BlockByHash {
	return &BlockByHash{
		blockStore: blockStore,
		adapter:    adapter,
	}
}

func (e *BlockByHash) GetBlockByHash(hash common.Hash, inclTx bool) (map[string]any, error) {
	block := e.blockStore.BlockByHash(hash)
	if block == nil {
		return nil, ethereum.NotFound
	}
	txs, err := e.adapter(block.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt cosmos txs: %v", err)
	}
	return block.ToEthLikeBlock(txs, inclTx), nil
}
