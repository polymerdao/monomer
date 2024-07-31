package builder

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/evm"
	rollupv1 "github.com/polymerdao/monomer/gen/rollup/v1"
	"github.com/polymerdao/monomer/mempool"
)

type Builder struct {
	mempool    *mempool.Pool
	app        monomer.Application
	blockStore store.BlockStore
	txStore    txstore.TxStore
	eventBus   *bfttypes.EventBus
	chainID    monomer.ChainID
	ethstatedb state.Database
}

func New(
	mpool *mempool.Pool,
	app monomer.Application,
	blockStore store.BlockStore,
	txStore txstore.TxStore,
	eventBus *bfttypes.EventBus,
	chainID monomer.ChainID,
	ethstatedb state.Database,
) *Builder {
	return &Builder{
		mempool:    mpool,
		app:        app,
		blockStore: blockStore,
		txStore:    txStore,
		eventBus:   eventBus,
		chainID:    chainID,
		ethstatedb: ethstatedb,
	}
}

// Rollback rolls back the block store, tx store, and application.
// TODO does anything need to be done with the event bus?
// assumptions:
//   - all hashes exist in the block store.
//   - finalized.Height <= safe.Height <= head.Height
func (b *Builder) Rollback(ctx context.Context, head, safe, finalized common.Hash) error {
	headBlock := b.blockStore.HeadBlock()
	if headBlock == nil {
		return errors.New("head block not found")
	}
	currentHeight := headBlock.Header.Height

	block := b.blockStore.BlockByHash(head)
	if block == nil {
		return fmt.Errorf("block not found with hash %s", head)
	}
	targetHeight := block.Header.Height

	if err := b.blockStore.RollbackToHeight(targetHeight); err != nil {
		return fmt.Errorf("rollback block store: %v", err)
	}
	if err := b.blockStore.UpdateLabel(eth.Unsafe, head); err != nil {
		return fmt.Errorf("update unsafe label: %v", err)
	}
	if err := b.blockStore.UpdateLabel(eth.Safe, safe); err != nil {
		return fmt.Errorf("update safe label: %v", err)
	}
	if err := b.blockStore.UpdateLabel(eth.Finalized, finalized); err != nil {
		return fmt.Errorf("update finalized label: %v", err)
	}

	if err := b.txStore.RollbackToHeight(targetHeight, currentHeight); err != nil {
		return fmt.Errorf("rollback tx store: %v", err)
	}

	if err := b.app.RollbackToHeight(ctx, uint64(targetHeight)); err != nil {
		return fmt.Errorf("rollback app: %v", err)
	}

	return nil
}

type Payload struct {
	// InjectedTransactions functions as an inclusion list. It contains transactions
	// from the consensus layer that must be included in the block.
	InjectedTransactions bfttypes.Txs
	// TODO: make the gas limit actually be enforced. Need to translate between cosmos and op gas limit.
	GasLimit  uint64
	Timestamp uint64
	NoTxPool  bool
}

func (b *Builder) Build(ctx context.Context, payload *Payload) (*monomer.Block, error) {
	txs := slices.Clone(payload.InjectedTransactions) // Shallow clone is ok, we just don't want to modify the slice itself.
	if !payload.NoTxPool {
		for {
			// TODO there is risk of losing txs if mempool db fails.
			// we need to fix db consistency in general, so we're just panicing on errors for now.
			length, err := b.mempool.Len()
			if err != nil {
				panic(fmt.Errorf("enqueue: %v", err))
			}
			if length == 0 {
				break
			}

			tx, err := b.mempool.Dequeue()
			if err != nil {
				panic(fmt.Errorf("dequeue: %v", err))
			}
			txs = append(txs, tx)
		}
	}

	// Build header.
	info, err := b.app.Info(ctx, &abcitypes.RequestInfo{})
	if err != nil {
		return nil, fmt.Errorf("info: %v", err)
	}
	currentHeight := info.GetLastBlockHeight()
	currentHead := b.blockStore.BlockByNumber(currentHeight)
	if currentHead == nil {
		return nil, fmt.Errorf("block not found at height: %d", currentHeight)
	}
	header := &monomer.Header{
		ChainID:    b.chainID,
		Height:     currentHeight + 1,
		Time:       payload.Timestamp,
		ParentHash: currentHead.Header.Hash,
		GasLimit:   payload.GasLimit,
	}

	cometHeader := header.ToComet()
	cometHeader.AppHash = info.GetLastBlockAppHash()
	resp, err := b.app.FinalizeBlock(ctx, &abcitypes.RequestFinalizeBlock{
		Txs:                txs.ToSliceOfBytes(),
		Hash:               cometHeader.Hash(),
		Height:             cometHeader.Height,
		Time:               cometHeader.Time,
		NextValidatorsHash: cometHeader.NextValidatorsHash,
		ProposerAddress:    cometHeader.ProposerAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("finalize block: %v", err)
	}
	_, err = b.app.Commit(ctx, &abcitypes.RequestCommit{})
	if err != nil {
		return nil, fmt.Errorf("commit: %v", err)
	}

	ethState, err := state.New(b.blockStore.HeadBlock().Header.StateRoot, b.ethstatedb, nil)
	if err != nil {
		return nil, fmt.Errorf("create ethereum state: %v", err)
	}
	// Store the updated cosmos app hash in the monomer EVM state db.
	if err := b.storeAppHashInEVM(resp.AppHash, ethState, header); err != nil {
		return nil, fmt.Errorf("store app hash in EVM: %v", err)
	}

	execTxResults := resp.GetTxResults()
	txResults := make([]*abcitypes.TxResult, 0, len(execTxResults))
	for i, execTxResult := range execTxResults {
		tx := txs[i]
		txResults = append(txResults, &abcitypes.TxResult{
			Height: header.Height,
			Index:  uint32(i),
			// This should work https://docs.cometbft.com/v0.38/spec/abci/abci++_methods#finalizeblock
			// The application shouldn't return the execTxResults in a different order than the corresponding txs.
			Tx:     tx,
			Result: *execTxResult,
		})

		cosmosTx := new(sdktx.Tx)
		if err := cosmosTx.Unmarshal(tx); err != nil {
			return nil, fmt.Errorf("unmarshal cosmos tx: %v", err)
		}
		// TODO: will the withdrawal msg always be the first tx message or do all tx messages need to be checked?
		for _, msg := range cosmosTx.GetBody().GetMessages() {
			withdrawalMsg := new(rollupv1.InitiateWithdrawalRequest)
			// TODO: is there a better way to check if a message is a withdrawal message type?
			if err := withdrawalMsg.Unmarshal(msg.GetValue()); err != nil {
				continue
			}
			if err := b.storeWithdrawalMsgInEVM(withdrawalMsg, ethState, header); err != nil {
				return nil, fmt.Errorf("store withdrawal msg in EVM: %v", err)
			}
		}
	}

	ethStateRoot, err := ethState.Commit(uint64(header.Height), true)
	if err != nil {
		return nil, fmt.Errorf("commit ethereum state: %v", err)
	}
	header.StateRoot = ethStateRoot

	block, err := monomer.MakeBlock(header, txs)
	if err != nil {
		return nil, fmt.Errorf("make block: %v", err)
	}

	// Append block.
	b.blockStore.AddBlock(block)

	// Index txs.
	if err := b.txStore.Add(txResults); err != nil {
		return nil, fmt.Errorf("add tx results: %v", err)
	}
	// Publish events.
	for _, txResult := range txResults {
		if err := b.eventBus.PublishEventTx(bfttypes.EventDataTx{
			TxResult: *txResult,
		}); err != nil {
			return nil, fmt.Errorf("publish event tx: %v", err)
		}
	}

	// TODO publish other things like new blocks.
	return block, nil
}

// storeAppHashInEVM stores the updated cosmos app hash in the monomer EVM state db. This is used for proving withdrawals.
func (b *Builder) storeAppHashInEVM(appHash []byte, ethState *state.StateDB, header *monomer.Header) error {
	monomerEVM, err := evm.NewEVM(ethState, header, b.chainID.Big())
	if err != nil {
		return fmt.Errorf("new EVM: %v", err)
	}
	executer, err := bindings.NewL2ApplicationStateRootProviderExecuter(monomerEVM)
	if err != nil {
		return fmt.Errorf("new L2ApplicationStateRootProviderExecuter: %v", err)
	}

	if err := executer.SetL2ApplicationStateRoot(common.BytesToHash(appHash)); err != nil {
		return fmt.Errorf("set L2ApplicationStateRoot: %v", err)
	}

	return nil
}

// storeWithdrawalMsgInEVM stores the withdrawal message hash in the monomer evm state db. This is used for proving withdrawals.
func (b *Builder) storeWithdrawalMsgInEVM(withdrawalMsg *rollupv1.InitiateWithdrawalRequest, ethState *state.StateDB, header *monomer.Header) error {
	monomerEVM, err := evm.NewEVM(ethState, header, b.chainID.Big())
	if err != nil {
		return fmt.Errorf("new EVM: %v", err)
	}
	executer, err := bindings.NewL2ToL1MessagePasserExecuter(monomerEVM)
	if err != nil {
		return fmt.Errorf("new L2ToL1MessagePasserExecuter: %v", err)
	}

	if err := executer.InitiateWithdrawal(
		withdrawalMsg.GetSender(),
		withdrawalMsg.Amount.BigInt(),
		common.HexToAddress(withdrawalMsg.GetTarget()),
		new(big.Int).SetBytes(withdrawalMsg.GasLimit),
		withdrawalMsg.GetData(),
	); err != nil {
		return fmt.Errorf("initiate withdrawal: %v", err)
	}

	return nil
}
