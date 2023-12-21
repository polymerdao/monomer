package engine

import (
	"fmt"
	"github.com/armon/go-metrics"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"log"
	"math/big"
	"strconv"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type Hash = eetypes.Hash
type Block = eetypes.Block

type Node interface {
	LastBlockHeight() int64
	// SavePayload saves the payload by its ID if it's not already in payload cache.
	// Also update the latest Payload if this is a new payload
	SavePayload(payload *eetypes.Payload)
	GetPayload(payloadID eetypes.PayloadID) (*eetypes.Payload, bool)
	CurrentPayload() *eetypes.Payload
	// The latest unsafe block hash
	//
	// The latest unsafe block refers to sealed blocks, not the one that's being built on
	HeadBlockHash() Hash
	CommitBlock() error
	// GetETH returns the wrapped ETH balance in Wei of the given EVM address.
	GetETH(address common.Address, height int64) (*big.Int, error)
	GetChainID() string

	GetBlock(id any) (*Block, error)
	UpdateLabel(label eth.BlockLabel, hash Hash) error

	Rollback(head, safe, finalized *Block) error
}

// The public rpc methods are prefixed by the namespace (lower case) followed by all exported
// methods of the "service" in camelcase
func GetExecutionEngineAPIs(node Node, enabledApis server.ApiEnabledMask, logger server.Logger) []rpc.API {
	apis := []rpc.API{
		{
			Namespace: "engine",
			Service:   &engineAPIserver{node: node, logger: logger},
		}, {
			Namespace: "eth",
			Service:   &ethLikeServer{node: node, logger: logger},
		}, {
			Namespace: "pep",
			Service:   &peptideServer{node: node, logger: logger},
		},
	}
	if enabledApis.IsAdminApiEnabled() {
		apis = append(apis, rpc.API{
			Namespace: "admin",
			Service:   &adminServer{node: node, logger: logger},
		})
	}
	return apis
}

type engineAPIserver struct {
	node   Node
	logger server.Logger
}

func (e *engineAPIserver) rollback(head *Block, safeHash, finalizedHash eetypes.Hash) error {
	getId := func(label string, hash eetypes.Hash) any {
		if hash != eetypes.ZeroHash {
			return hash.Bytes()
		}
		return label
	}
	safe, err := e.node.GetBlock(getId(eth.Safe, safeHash))
	if err != nil {
		return err
	}
	finalized, err := e.node.GetBlock(getId(eth.Finalized, finalizedHash))
	if err != nil {
		return err
	}
	return e.node.Rollback(head, safe, finalized)
}

func (e *engineAPIserver) ForkchoiceUpdatedV1(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes) (*eth.ForkchoiceUpdatedResult, error) {
	e.logger.Debug("ForkchoiceUpdatedV1",
		"height", e.node.LastBlockHeight()+1,
		"unsafe", fcs.HeadBlockHash.Hex(),
		"safe", fcs.SafeBlockHash.Hex(),
		"finalized", fcs.FinalizedBlockHash.Hex(),
		"attr", eetypes.HasPayloadAttributes(&pa),
	)
	telemetry.IncrCounter(1, "query", "ForkchoiceUpdatedV1")

	if eetypes.IsForkchoiceStateEmpty(&fcs) {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("forkchoice state is empty"))
	}

	// update labeled blocks

	headBlock, err := e.node.GetBlock(fcs.HeadBlockHash.Bytes())
	if err != nil {
		return nil, engine.InvalidForkChoiceState.With(err)
	}

	if headBlock.Height() != e.node.LastBlockHeight() {
		e.logger.Error("block head does not match the last sealed block", "head_height", headBlock.Height(), "app_height", e.node.LastBlockHeight())
		if err := e.rollback(headBlock, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			e.logger.Error("rollback failed", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// update canonical block head
	e.node.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash)

	if fcs.SafeBlockHash != eetypes.ZeroHash {
		e.logger.Info("updating safe block", "hash", fcs.SafeBlockHash)
		if err := e.node.UpdateLabel(eth.Safe, fcs.SafeBlockHash); err != nil {
			e.logger.Error("invalid safe head", "err", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	if fcs.FinalizedBlockHash != eetypes.ZeroHash {
		e.logger.Info("updating finalized block", "hash", fcs.FinalizedBlockHash)
		if err := e.node.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash); err != nil {
			e.logger.Error("invalid finalized head", "err", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// start new payload mode
	if eetypes.HasPayloadAttributes(&pa) {
		// TODO check for invalid txs in pa
		payload := eetypes.NewPayload(&pa, fcs.HeadBlockHash, e.node.LastBlockHeight()+1)
		payloadId, err := payload.GetPayloadID()

		if err != nil {
			return nil, engine.InvalidPayloadAttributes.With(err)
		}
		e.node.SavePayload(payload)
		return payload.Valid(payloadId), nil
	}

	return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, nil), nil
}

func (e *engineAPIserver) GetPayloadV1(payloadID eetypes.PayloadID) (*eth.ExecutionPayload, error) {
	e.logger.Debug("GetPayloadV1", "payload_id", payloadID, "height", e.node.LastBlockHeight()+1)
	telemetry.IncrCounter(1, "query", "GetPayloadV1")

	payload, ok := e.node.GetPayload(payloadID)
	if !ok {
		return nil, eetypes.UnknownPayload
	}
	if payload != e.node.CurrentPayload() {
		return nil, engine.InvalidParams.With(fmt.Errorf("payload is not current"))
	}

	// e.mutex.Lock()
	// defer e.mutex.Unlock()

	// e.debugL1UserTxs(payload.Attrs.Transactions, "EngineGetPayload")

	// TODO: handle time slot based block production
	// for now assume block is sealed by this call
	err := e.node.CommitBlock()
	// TODO error handling
	if err != nil {
		e.logger.Error("failed to commit block", "err", err)
		log.Panicf("failed to commit block: %v", err)
	}

	// return payload.ToExecutionPayload(e.latestBlock.Hash()), nil
	return payload.ToExecutionPayload(e.node.HeadBlockHash()), nil
}

func (e *engineAPIserver) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.logger.Debug("NewPayloadV1", "payload.ID", payload.ID(), "height", e.node.LastBlockHeight()+1)
	if _, err := e.node.GetBlock(payload.BlockHash.Bytes()); err != nil {
		return &eth.PayloadStatusV1{Status: eth.ExecutionInvalidBlockHash},
			engine.InvalidParams.With(err)
	}
	headBlockHash := e.node.HeadBlockHash()
	return &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, nil
}

// ------------------------------------------------------------------------------------------------------------------

type ethLikeServer struct {
	node   Node
	logger server.Logger
}

func (e *ethLikeServer) GetProof(address common.Address, storage []Hash, blockTag string) (*eth.AccountResult, error) {
	e.logger.Debug("GetProof", "address", address, "storage", storage, "blockTag", blockTag)
	telemetry.IncrCounter(1, "query", "GetProof")

	return &eth.AccountResult{}, nil
}

func (e *ethLikeServer) ChainId() hexutil.Big {
	e.logger.Debug("ChainId")
	telemetry.IncrCounter(1, "query", "ChainId")

	chainID, ok := new(big.Int).SetString(e.node.GetChainID(), 10)
	if !ok {
		panic("chain id is not numerical")
	}
	return (hexutil.Big)(*chainID)
}

// GetBalance returns wrapped Ethers balance on L2 chain
// - address: EVM address
// - blockNumber: a valid BlockLabel or hex encoded big.Int; default to latest/unsafe block
func (e *ethLikeServer) GetBalance(address common.Address, id any) (hexutil.Big, error) {
	e.logger.Debug("GetBalance", "address", address, "id", id)
	telemetry.IncrCounter(1, "query", "GetBalance")

	b, err := e.node.GetBlock(id)
	if err != nil {
		return hexutil.Big{}, err
	}

	balance, err := e.node.GetETH(address, b.Height())
	if err != nil {
		err = fmt.Errorf("failed to get balance for address %s at block height %d, %w", address, b.Height(), err)
		return hexutil.Big{}, err
	}
	return (hexutil.Big)(*balance), nil
}

func (e *ethLikeServer) GetBlockByHash(hash Hash, inclTx bool) (map[string]any, error) {
	e.logger.Debug("GetBlockByHash", "hash", hash.Hex(), "inclTx", inclTx)
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByHash"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b, err := e.node.GetBlock(hash.Bytes())
	if err != nil {
		return nil, err
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *ethLikeServer) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	e.logger.Debug("GetBlockByNumber", "id", id, "inclTx", inclTx)
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByNumber"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b, err := e.node.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return b.ToEthLikeBlock(inclTx), nil
}

// ------------------------------------------------------------------------------------------------------------------

type peptideServer struct {
	node   Node
	logger server.Logger
}

func (e *peptideServer) GetBlock(id any) (*Block, error) {
	e.logger.Debug("GetBlock", "id", id)
	telemetry.IncrCounter(1, "query", "GetBlock")

	block, err := e.node.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return block, nil
}

// ------------------------------------------------------------------------------------------------------------------

// admin API
type adminServer struct {
	node   Node
	logger server.Logger
}

func (e *adminServer) Rollback(headHeight, safeHeight, finalizedHeight int64) error {
	e.logger.Debug("Rollback", "head", headHeight, "safe", safeHeight, "finalized", finalizedHeight)
	head, err := e.node.GetBlock(headHeight)
	if err != nil {
		return err
	}
	safe, err := e.node.GetBlock(safeHeight)
	if err != nil {
		return err
	}
	finalized, err := e.node.GetBlock(finalizedHeight)
	if err != nil {
		return err
	}

	return e.node.Rollback(head, safe, finalized)
}
