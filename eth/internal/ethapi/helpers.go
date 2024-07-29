package ethapi

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/polymerdao/monomer/utils"
)

type noopBackend struct{}

func (noopBackend) GetReceipts(_ context.Context, _ common.Hash) (types.Receipts, error) {
	return nil, nil
}

func SimpleRPCMarshalBlock(block *types.Block, fullTx bool, chainID *big.Int) (map[string]interface{}, error) {
	return RPCMarshalBlock(context.Background(), block, true, fullTx, &params.ChainConfig{
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
	}, noopBackend{})
}
