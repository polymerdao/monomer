package ethapi

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
)

type noopBackend struct{}

func (noopBackend) ChainConfig() *params.ChainConfig {
	return nil
}

func (noopBackend) GetReceipts(_ context.Context, _ common.Hash) (types.Receipts, error) {
	return nil, nil
}

func (noopBackend) HeaderByNumberOrHash(_ context.Context, _ rpc.BlockNumberOrHash) (*types.Header, error) {
	return nil, nil
}

func (noopBackend) HistoricalRPCService() *rpc.Client {
	return nil
}

func (noopBackend) StateAndHeaderByNumberOrHash(_ context.Context, _ rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	return nil, nil, nil
}

func SimpleRPCMarshalBlock(block *types.Block, fullTx bool, chainID *big.Int) (map[string]interface{}, error) {
	return RPCMarshalBlock(context.Background(), block, true, fullTx, monomer.NewChainConfig(chainID), noopBackend{})
}
