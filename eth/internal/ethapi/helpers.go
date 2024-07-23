package ethapi

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type noopBackend struct{}

func (noopBackend) GetReceipts(_ context.Context, _ common.Hash) (types.Receipts, error) {
	return nil, nil
}

func SimpleRPCMarshalBlock(block *types.Block, fullTx bool) (map[string]interface{}, error) {
	return RPCMarshalBlock(context.Background(), block, true, fullTx, &params.ChainConfig{}, noopBackend{})
}
