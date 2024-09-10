package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
	"github.com/polymerdao/monomer/monomerdb"
)

type ChainIDAPI struct {
	chainID *hexutil.Big
	metrics Metrics
}

func NewChainIDAPI(chainID *hexutil.Big, metrics Metrics) *ChainIDAPI {
	return &ChainIDAPI{
		chainID: chainID,
		metrics: metrics,
	}
}

func (e *ChainIDAPI) ChainId() *hexutil.Big { //nolint:stylecheck
	defer e.metrics.RecordRPCMethodCall(ChainIDMethodName, time.Now())

	return e.chainID
}

type DB interface {
	BlockByLabel(eth.BlockLabel) (*monomer.Block, error)
	BlockByHeight(uint64) (*monomer.Block, error)
	BlockByHash(common.Hash) (*monomer.Block, error)
	HeadBlock() (*monomer.Block, error)
}

type BlockAPI struct {
	blockStore DB
	chainID    *big.Int
	metrics    Metrics
}

func NewBlockAPI(blockStore DB, chainID *big.Int, metrics Metrics) *BlockAPI {
	return &BlockAPI{
		blockStore: blockStore,
		chainID:    chainID,
		metrics:    metrics,
	}
}

func (e *BlockAPI) GetBlockByNumber(id BlockID, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByNumberMethodName, time.Now())

	block, err := id.Get(e.blockStore)
	if errors.Is(err, monomerdb.ErrNotFound) {
		return nil, ethereum.NotFound
	} else if err != nil {
		return nil, err
	}
	return e.toRPCBlock(block, fullTx)
}

func (e *BlockAPI) GetBlockByHash(hash common.Hash, fullTx bool) (map[string]any, error) {
	defer e.metrics.RecordRPCMethodCall(GetBlockByHashMethodName, time.Now())

	block, err := e.blockStore.BlockByHash(hash)
	if errors.Is(err, monomerdb.ErrNotFound) {
		return nil, ethereum.NotFound
	} else if err != nil {
		return nil, err
	}
	return e.toRPCBlock(block, fullTx)
}

func (e *BlockAPI) toRPCBlock(block *monomer.Block, fullTx bool) (map[string]any, error) {
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

type ProofAPI struct {
	blockchainAPI *ethapi.BlockChainAPI
}

func NewProofAPI(db state.Database, blockStore DB) *ProofAPI {
	return &ProofAPI{
		blockchainAPI: ethapi.NewBlockChainAPI(newEthAPIBackend(db, blockStore)),
	}
}

// GetProof returns the account and storage values of the specified account including the Merkle-proof.
// The user can specify either a block number or a block hash to build to proof from.
func (p *ProofAPI) GetProof(
	ctx context.Context,
	address common.Address,
	storageKeys []string,
	blockNrOrHash rpc.BlockNumberOrHash,
) (*ethapi.AccountResult, error) {
	return p.blockchainAPI.GetProof(ctx, address, storageKeys, blockNrOrHash)
}
