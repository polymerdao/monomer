package builder

import (
	"context"
	"fmt"
	"math/big"
	"slices"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/bindings"
	"github.com/polymerdao/monomer/evm"
	"github.com/polymerdao/monomer/gen/rollup/v1"
	"github.com/polymerdao/monomer/mempool"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
)

type DB interface {
	Height() (uint64, error)
	HeaderByHash(hash common.Hash) (*monomer.Header, error)
	Rollback(unsafe, safe, finalized common.Hash) error
	HeaderByHeight(height uint64) (*monomer.Header, error)
	HeadHeader() (*monomer.Header, error)
	AppendBlock(*monomer.Block) error
}

type Builder struct {
	mempool    *mempool.Pool
	app        monomer.Application
	blockStore DB
	txStore    txstore.TxStore
	eventBus   *bfttypes.EventBus
	chainID    monomer.ChainID
	ethstatedb state.Database
}

func New(
	mpool *mempool.Pool,
	app monomer.Application,
	blockStore DB,
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
func (b *Builder) Rollback(ctx context.Context, unsafe, safe, finalized common.Hash) error {
	currentHeight, err := b.blockStore.Height()
	if err != nil {
		return fmt.Errorf("get height: %v", err)
	}

	unsafeHeader, err := b.blockStore.HeaderByHash(unsafe)
	if err != nil {
		return fmt.Errorf("get unsafe header: %v", err)
	}
	targetHeight := unsafeHeader.Height

	if err := b.blockStore.Rollback(unsafe, safe, finalized); err != nil {
		return fmt.Errorf("rollback block store: %v", err)
	}

	if err := b.txStore.RollbackToHeight(targetHeight, int64(currentHeight)); err != nil {
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
	currentHeader, err := b.blockStore.HeadHeader()
	if err != nil {
		return nil, fmt.Errorf("header by height: %v", err)
	}
	header := &monomer.Header{
		ChainID:    b.chainID,
		Height:     currentHeader.Height + 1,
		Time:       payload.Timestamp,
		ParentHash: currentHeader.Hash,
		GasLimit:   payload.GasLimit,
	}

	cometHeader := header.ToComet()
	info, err := b.app.Info(ctx, &abcitypes.RequestInfo{})
	if err != nil {
		return nil, fmt.Errorf("info: %v", err)
	}
	cometHeader.AppHash = info.GetLastBlockAppHash() // TODO maybe best to get this from the ethstatedb?
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

	ethState, err := state.New(currentHeader.StateRoot, b.ethstatedb, nil)
	if err != nil {
		return nil, fmt.Errorf("create ethereum state: %v", err)
	}
	// Store the updated cosmos app hash in the monomer EVM state db.
	if err = b.storeAppHashInEVM(resp.AppHash, ethState, header); err != nil {
		return nil, fmt.Errorf("store app hash in EVM: %v", err)
	}

	execTxResults := resp.GetTxResults()
	txResults := make([]*abcitypes.TxResult, 0, len(execTxResults))
	for i, execTxResult := range execTxResults {
		tx := txs[i]

		// Check for withdrawal messages in the tx.
		execTxResult, err = b.parseWithdrawalMessages(tx, execTxResult, ethState, header)
		if err != nil {
			return nil, fmt.Errorf("parse withdrawal messages: %v", err)
		}

		txResults = append(txResults, &abcitypes.TxResult{
			Height: header.Height,
			Index:  uint32(i),
			// This should work https://docs.cometbft.com/v0.38/spec/abci/abci++_methods#finalizeblock
			// The application shouldn't return the execTxResults in a different order than the corresponding txs.
			Tx:     tx,
			Result: *execTxResult,
		})
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
	if err := b.blockStore.AppendBlock(block); err != nil {
		return nil, fmt.Errorf("append block: %v", err)
	}

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
	monomerEVM, err := evm.NewEVM(ethState, header)
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

// parseWithdrawalMessages checks for withdrawal messages if the tx was successful. If a withdrawal message is found, the
// message nonce is appended to the withdrawal message event attributes and the updated execTxResult is returned.
func (b *Builder) parseWithdrawalMessages(
	tx bfttypes.Tx,
	execTxResult *abcitypes.ExecTxResult,
	ethState *state.StateDB,
	header *monomer.Header,
) (*abcitypes.ExecTxResult, error) {
	if execTxResult.IsOK() {
		cosmosTx := new(sdktx.Tx)
		if err := cosmosTx.Unmarshal(tx); err != nil {
			return nil, fmt.Errorf("unmarshal cosmos tx: %v", err)
		}
		for _, msg := range cosmosTx.GetBody().GetMessages() {
			withdrawalMsg := new(rollupv1.InitiateWithdrawalRequest)
			if msg.TypeUrl == cdctypes.MsgTypeURL(withdrawalMsg) {
				if err := withdrawalMsg.Unmarshal(msg.GetValue()); err != nil {
					return nil, fmt.Errorf("unmarshal InitiateWithdrawalRequest: %v", err)
				}

				// Store the withdrawal message hash in the monomer EVM state db.
				nonce, err := b.storeWithdrawalMsgInEVM(withdrawalMsg, ethState, header)
				if err != nil {
					return nil, fmt.Errorf("store withdrawal msg in EVM: %v", err)
				}

				// Populate the nonce in the tx event attributes.
				for _, event := range execTxResult.GetEvents() {
					if event.GetType() == rolluptypes.EventTypeWithdrawalInitiated {
						event.Attributes = append(event.Attributes, abcitypes.EventAttribute{
							Key:   rolluptypes.AttributeKeyNonce,
							Value: hexutil.Encode(nonce.Bytes()),
						})
					}
				}
			}
		}
	}
	return execTxResult, nil
}

// storeWithdrawalMsgInEVM stores the withdrawal message hash in the monomer evm state db and returns the L2ToL1MessagePasser
// message nonce used for the withdrawal. This is used for proving withdrawals.
func (b *Builder) storeWithdrawalMsgInEVM(
	withdrawalMsg *rollupv1.InitiateWithdrawalRequest,
	ethState *state.StateDB,
	header *monomer.Header,
) (*big.Int, error) {
	monomerEVM, err := evm.NewEVM(ethState, header)
	if err != nil {
		return nil, fmt.Errorf("new EVM: %v", err)
	}
	executer, err := bindings.NewL2ToL1MessagePasserExecuter(monomerEVM)
	if err != nil {
		return nil, fmt.Errorf("new L2ToL1MessagePasserExecuter: %v", err)
	}

	// Get the current message nonce before initiating the withdrawal.
	messageNonce, err := executer.GetMessageNonce()
	if err != nil {
		return nil, fmt.Errorf("get message nonce: %v", err)
	}

	// Initiate the withdrawal in the Monomer ethereum state.
	if err = executer.InitiateWithdrawal(
		withdrawalMsg.GetSender(),
		withdrawalMsg.Value.BigInt(),
		common.HexToAddress(withdrawalMsg.GetTarget()),
		new(big.Int).SetBytes(withdrawalMsg.GasLimit),
		withdrawalMsg.GetData(),
	); err != nil {
		return nil, fmt.Errorf("initiate withdrawal: %v", err)
	}

	return messageNonce, nil
}
