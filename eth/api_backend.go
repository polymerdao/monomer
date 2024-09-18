package eth

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
)

type ethAPIBackend struct {
	database   state.Database
	blockStore DB
}

func newEthAPIBackend(db state.Database, blockStore DB) *ethAPIBackend {
	return &ethAPIBackend{
		database:   db,
		blockStore: blockStore,
	}
}

func (*ethAPIBackend) ChainConfig() *params.ChainConfig {
	return monomer.NewChainConfig(nil)
}

func (*ethAPIBackend) GetReceipts(_ context.Context, _ common.Hash) (types.Receipts, error) {
	return nil, nil
}

func (b *ethAPIBackend) HeaderByNumberOrHash(_ context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	var (
		block *monomer.Block
		err   error
	)
	if blockNr, ok := blockNrOrHash.Number(); ok {
		if blockNr == rpc.LatestBlockNumber {
			block, err = b.blockStore.HeadBlock()
		} else if blockNr == rpc.EarliestBlockNumber || blockNr == rpc.PendingBlockNumber ||
			blockNr == rpc.SafeBlockNumber || blockNr == rpc.FinalizedBlockNumber {
			return nil, fmt.Errorf("\"%s\" is not supported; either use \"latest\" or a valid block number or hash", blockNr)
		} else {
			block, err = b.blockStore.BlockByHeight(uint64(blockNr))
		}
		if err != nil {
			return nil, fmt.Errorf("get block by height (%d): %w", blockNr, err)
		}
	} else if hash, ok := blockNrOrHash.Hash(); ok {
		block, err = b.blockStore.BlockByHash(hash)
		if err != nil {
			return nil, fmt.Errorf("get block by hash (%s): %w", hash, err)
		}
	}

	if block == nil {
		return nil, fmt.Errorf("block %w", ethereum.NotFound)
	}
	header := block.Header
	if header == nil {
		return nil, fmt.Errorf("header %w", ethereum.NotFound)
	}
	return header.ToEth(), nil
}

func (*ethAPIBackend) HistoricalRPCService() *rpc.Client {
	return nil
}

func (b *ethAPIBackend) StateAndHeaderByNumberOrHash(
	ctx context.Context,
	blockNrOrHash rpc.BlockNumberOrHash,
) (*state.StateDB, *types.Header, error) {
	header, err := b.HeaderByNumberOrHash(ctx, blockNrOrHash)
	if err != nil {
		return nil, nil, fmt.Errorf("header by number or hash: %w", err)
	}

	stateDB, err := state.New(header.Root, b.database, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("opening state.StateDB: %w", err)
	}
	if stateDB == nil {
		return nil, nil, fmt.Errorf("stateDB %w", ethereum.NotFound)
	}

	return stateDB, header, nil
}
