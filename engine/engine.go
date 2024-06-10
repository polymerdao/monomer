package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/builder"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

type BlockStore interface {
	store.BlockStoreReader
	UpdateLabel(label eth.BlockLabel, hash common.Hash) error
}

// EngineAPI implements the Engine API. It assumes it is the sole block proposer.
type EngineAPI struct {
	builder                  *builder.Builder
	txValidator              TxValidator
	blockStore               BlockStore
	currentPayloadAttributes *monomer.PayloadAttributes
	lock                     sync.RWMutex
}

type TxValidator interface {
	CheckTx(context.Context, *abci.RequestCheckTx) (*abci.ResponseCheckTx, error)
}

func NewEngineAPI(
	b *builder.Builder,
	txValidator TxValidator,
	blockStore BlockStore,
) *EngineAPI {
	return &EngineAPI{
		txValidator: txValidator,
		blockStore:  blockStore,
		builder:     b,
	}
}

func (e *EngineAPI) ForkchoiceUpdatedV1(
	ctx context.Context,
	fcs eth.ForkchoiceState, //nolint:gocritic
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	// TODO should this be called after Ecotone?
	return e.ForkchoiceUpdatedV3(ctx, fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV2(
	ctx context.Context,
	fcs eth.ForkchoiceState, //nolint:gocritic
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	// TODO should this be called after Ecotone?
	return e.ForkchoiceUpdatedV3(ctx, fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV3(
	ctx context.Context,
	fcs eth.ForkchoiceState, //nolint:gocritic
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	// OP spec:
	//   - headBlockHash: block hash of the head of the canonical chain. Labeled "unsafe" in user JSON-RPC.
	//     Nodes may apply L2 blocks out of band ahead of time, and then reorg when L1 data conflicts.
	//   - safeBlockHash: block hash of the canonical chain, derived from L1 data, unlikely to reorg.
	//   - finalizedBlockHash: irreversible block hash, matches lower boundary of the dispute period.

	headBlock := e.blockStore.BlockByHash(fcs.HeadBlockHash)

	// Engine API spec:
	//   Before updating the forkchoice state, client software MUST ensure the validity of the payload referenced by
	//   forkchoiceState.headBlockHash...
	// Because we assume we're the only proposer, this is equivalent to checking if the head block is present in the block store.
	if headBlock == nil {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("head block not found"))
	}

	// Engine API spec:
	//   Client software MUST return -38002: Invalid forkchoice state error if the payload referenced by forkchoiceState.headBlockHash
	//   is VALID and a payload referenced by either forkchoiceState.finalizedBlockHash or forkchoiceState.safeBlockHash does not
	//   belong to the chain defined by forkchoiceState.headBlockHash.
	if safeBlock := e.blockStore.BlockByHash(fcs.SafeBlockHash); safeBlock == nil {
		return nil, engine.InvalidPayloadAttributes.With(errors.New("safe block not found"))
	} else if safeBlock.Header.Height > headBlock.Header.Height {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("safe block at height %d comes after head block at height %d",
			safeBlock.Header.Height, headBlock.Header.Height))
	}
	if finalizedBlock := e.blockStore.BlockByHash(fcs.FinalizedBlockHash); finalizedBlock == nil {
		return nil, engine.InvalidPayloadAttributes.With(errors.New("finalized block not found"))
	} else if finalizedBlock.Header.Height > headBlock.Header.Height {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf(
			"finalized block at height %d comes after head block at height %d", finalizedBlock.Header.Height,
			headBlock.Header.Height))
	}

	// Engine API spec:
	//   Client software MAY skip an update of the forkchoice state and MUST NOT begin a payload build process if
	//   forkchoiceState.headBlockHash references an ancestor of the head of canonical chain.
	// This part of the spec does not apply to us.
	// Because we assume we're the sole proposer, the CL should only give us a past block head hash when L1 reorgs.
	// TODO Is reorg handling in the Engine API discussed in the OP Execution Engine spec?
	if headBlock.Header.Height < e.blockStore.HeadBlock().Header.Height {
		if err := e.builder.Rollback(ctx, fcs.HeadBlockHash, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			return nil, engine.GenericServerError.With(fmt.Errorf("rollback: %v", err))
		}
	}

	// Update block labels.
	if err := e.blockStore.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash); err != nil {
		return nil, engine.GenericServerError.With(fmt.Errorf("update unsafe label: %v", err))
	}
	if err := e.blockStore.UpdateLabel(eth.Safe, fcs.SafeBlockHash); err != nil {
		return nil, engine.GenericServerError.With(fmt.Errorf("update safe label: %v", err))
	}
	if err := e.blockStore.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash); err != nil {
		return nil, engine.GenericServerError.With(fmt.Errorf("update finalized label: %v", err))
	}

	if pa == nil {
		// Engine API spec:
		//   `payloadId: null`... if the payload is deemed VALID and a build process hasn't been started.
		return monomer.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, nil), nil
	}

	// Ethereum execution specs:
	//   https://github.com/ethereum/execution-specs/blob/119208cf1a13d5002074bcee3b8ea4ef096eeb0d/src/ethereum/shanghai/fork.py#L298
	if headTime := e.blockStore.HeadBlock().Header.Time; uint64(pa.Timestamp) <= headTime {
		return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("timestamp too small: parent timestamp %d, got %d", headTime,
			pa.Timestamp))
	}

	// Docs on OP PayloadAttributes struct:
	//   Withdrawals... should be nil or empty depending on Shanghai enablement
	//   Starting at Ecotone, the parentBeaconBlockRoot must be set to the L1 origin parentBeaconBlockRoot, or a zero bytes32 if the
	//   Dencun functionality with parentBeaconBlockRoot is not active on L1.
	// We don't make any judgements about what hard fork is on L1.
	// We can change this later if it becomes an issue, but right now it just prevents us from using Geth in PoW clique mode for
	// devnets.

	// OP Spec:
	//   The gasLimit is optional w.r.t. compatibility with L1, but required when used as rollup.
	//   This field overrides the gas limit used during block-building. If not specified as rollup, a STATUS_INVALID is returned.
	// Monomer is always used as a rollup.
	// I do not know how to reconcile the above with:
	// Engine API spec:
	//   Client software MUST respond to this method call in the following way: ...
	//     [InvalidPayloadAttributes] if the payload is deemed VALID and forkchoiceState has been applied successfully, but no build
	//     process has been started due to invalid payloadAttributes.
	// STATUS_INVALID is only for applying the head block payload and executing and checking the payload transactions.
	// OP-Geth returns InvalidParams.
	if pa.GasLimit == nil {
		return nil, engine.InvalidPayloadAttributes.With(errors.New("gas limit not provided"))
	}

	cosmosTxs, err := rolluptypes.AdaptPayloadTxsToCosmosTxs(pa.Transactions)
	if err != nil {
		return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("convert payload attributes txs to cosmos txs: %v", err))
	}
	if len(cosmosTxs) == 0 {
		return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("L1 Attributes tx not found"))
	}

	// OP Spec:
	//   If the transactions field is present, the engine must execute the transactions in order and return STATUS_INVALID if there is
	//   an error processing the transactions.
	//   It must return STATUS_VALID if all of the transactions could be executed without error.
	// TODO checktx doesn't actually run the tx, it only does basic validation.
	for _, txBytes := range cosmosTxs {
		if _, err := e.txValidator.CheckTx(context.Background(), &abci.RequestCheckTx{
			Tx: txBytes,
		}); err != nil {
			return &eth.ForkchoiceUpdatedResult{
				PayloadStatus: eth.PayloadStatusV1{
					Status:          eth.ExecutionInvalid,
					LatestValidHash: &fcs.HeadBlockHash,
				},
			}, nil
		}
	}

	// Engine API spec:
	//   Client software MUST begin a payload build process building on top of forkchoiceState.headBlockHash and identified via
	//   buildProcessId value if payloadAttributes is not null and the forkchoice state has been updated successfully.
	//
	// Monomer does not have an async build process. We store the payload for the next call to GetPayload.
	e.currentPayloadAttributes = &monomer.PayloadAttributes{
		Timestamp:             uint64(pa.Timestamp),
		PrevRandao:            pa.PrevRandao,
		SuggestedFeeRecipient: pa.SuggestedFeeRecipient,
		Withdrawals:           pa.Withdrawals,
		NoTxPool:              pa.NoTxPool,
		GasLimit:              uint64(*pa.GasLimit),
		ParentBeaconBlockRoot: pa.ParentBeaconBlockRoot,
		ParentHash:            fcs.HeadBlockHash,
		Height:                e.blockStore.HeadBlock().Header.Height + 1,
		Transactions:          pa.Transactions,
		CosmosTxs:             cosmosTxs,
	}

	// Engine API spec:
	//   latestValidHash: ... the hash of the most recent valid block in the branch defined by payload and its ancestors.
	// Recall that "payload" refers to the most recent block appended to the canonical chain, not the payload attributes.
	return monomer.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, e.currentPayloadAttributes.ID()), nil
}

func (e *EngineAPI) GetPayloadV1(ctx context.Context, payloadID engine.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	// TODO should this be called after Ecotone?
	return e.GetPayloadV3(ctx, payloadID)
}

func (e *EngineAPI) GetPayloadV2(ctx context.Context, payloadID engine.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	// TODO should this be called after Ecotone?
	return e.GetPayloadV3(ctx, payloadID)
}

// GetPayloadV3 seals a payload that is currently being built (i.e. was introduced in the PayloadAttributes from a previous
// ForkchoiceUpdated call).
func (e *EngineAPI) GetPayloadV3(ctx context.Context, payloadID engine.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	if e.currentPayloadAttributes == nil {
		return nil, engine.InvalidParams.With(errors.New("payload not found"))
	}

	if payloadID != *e.currentPayloadAttributes.ID() {
		return nil, engine.InvalidParams.With(errors.New("payload is not current"))
	}

	// TODO: handle time slot based block production
	// for now assume block is sealed by this call
	block, err := e.builder.Build(ctx, &builder.Payload{
		InjectedTransactions: e.currentPayloadAttributes.CosmosTxs,
		GasLimit:             e.currentPayloadAttributes.GasLimit,
		Timestamp:            e.currentPayloadAttributes.Timestamp,
		NoTxPool:             e.currentPayloadAttributes.NoTxPool,
	})
	if err != nil {
		log.Panicf("failed to commit block: %v", err) // TODO error handling. An error here is potentially a big problem.
	}

	txs, err := rolluptypes.AdaptCosmosTxsToEthTxs(block.Txs)
	if err != nil {
		return nil, engine.GenericServerError.With(fmt.Errorf("convert cosmos txs to eth txs: %v", err))
	}

	txBytes := make([]hexutil.Bytes, len(txs))
	for i, tx := range txs {
		txBytes[i], err = tx.MarshalBinary()
		if err != nil {
			return nil, engine.GenericServerError.With(fmt.Errorf("marshal tx binary: %v", err))
		}
	}

	payloadEnvelope := &eth.ExecutionPayloadEnvelope{
		ExecutionPayload: &eth.ExecutionPayload{
			ParentHash:   e.currentPayloadAttributes.ParentHash,
			BlockNumber:  hexutil.Uint64(e.currentPayloadAttributes.Height),
			BlockHash:    block.Hash(),
			FeeRecipient: e.currentPayloadAttributes.SuggestedFeeRecipient,
			Timestamp:    hexutil.Uint64(e.currentPayloadAttributes.Timestamp),
			PrevRandao:   e.currentPayloadAttributes.PrevRandao,
			Withdrawals:  e.currentPayloadAttributes.Withdrawals,
			Transactions: txBytes,
			GasLimit:     hexutil.Uint64(e.currentPayloadAttributes.GasLimit),
		},
	}
	// remove payload
	e.currentPayloadAttributes = nil

	return payloadEnvelope, nil
}

func (e *EngineAPI) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) { //nolint:gocritic
	// TODO should this be called after Ecotone?
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV2(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) { //nolint:gocritic
	// TODO should this be called after Ecotone?
	return e.NewPayloadV3(payload)
}

// NewPayloadV3 ensures the payload's block hash is present in the block store.
// TODO will this ever be called if we are the sole block proposer?
func (e *EngineAPI) NewPayloadV3(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) { //nolint:gocritic
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.blockStore.BlockByHash(payload.BlockHash) == nil {
		return &eth.PayloadStatusV1{
			Status: eth.ExecutionInvalidBlockHash,
		}, engine.InvalidParams.With(errors.New("block not found"))
	}
	headBlockHash := e.blockStore.HeadBlock().Hash()
	return &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, nil
}
