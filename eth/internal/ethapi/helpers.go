package ethapi

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/utils"
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

func NewChainConfig(chainID *big.Int) *params.ChainConfig {
	return &params.ChainConfig{
		ChainID: chainID,

		ByzantiumBlock:      new(big.Int),
		ConstantinopleBlock: new(big.Int),
		PetersburgBlock:     new(big.Int),
		IstanbulBlock:       new(big.Int),
		MuirGlacierBlock:    new(big.Int),
		BerlinBlock:         new(big.Int),
		LondonBlock:         new(big.Int),
		ArrowGlacierBlock:   new(big.Int),
		GrayGlacierBlock:    new(big.Int),
		MergeNetsplitBlock:  new(big.Int),

		BedrockBlock: new(big.Int),
		RegolithTime: utils.Ptr(uint64(0)),
		CanyonTime:   utils.Ptr(uint64(0)),
	}
}

func SimpleRPCMarshalBlock(block *types.Block, fullTx bool, chainID *big.Int) (map[string]interface{}, error) {
	return RPCMarshalBlock(context.Background(), block, true, fullTx, NewChainConfig(chainID), noopBackend{})
}
