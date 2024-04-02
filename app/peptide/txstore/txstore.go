package txstore

import (
	"context"
	"fmt"

	dbm "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmtquery "github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/state/txindex"
	"github.com/cometbft/cometbft/state/txindex/kv"
	"github.com/cometbft/cometbft/types"
)

type TxStore interface {
	// Retrieves a transaction by hash from the indexer
	Get(hash []byte) (*abcitypes.TxResult, error)

	// Searches transactions that follow the provided query
	Search(ctx context.Context, q *cmtquery.Query) ([]*abcitypes.TxResult, error)

	// Adds a list of transactions to the indexer
	Add(txs []*abcitypes.TxResult) error

	// Removes all transactions from the indexer that belong to blocks after height.
	RollbackToHeight(rollbackHeight, currentHeight int64) error
}

type txstore struct {
	db  dbm.DB
	idx txindex.TxIndexer
}

var _ TxStore = (*txstore)(nil)

func NewTxStore(db dbm.DB) TxStore {
	return &txstore{
		db:  db,
		idx: kv.NewTxIndex(db),
	}
}

func (t *txstore) Get(hash []byte) (*abcitypes.TxResult, error) {
	return t.idx.Get(hash)
}

func (t *txstore) Search(ctx context.Context, q *cmtquery.Query) ([]*abcitypes.TxResult, error) {
	return t.idx.Search(ctx, q)
}

func (t *txstore) Add(txs []*abcitypes.TxResult) error {
	batch := txindex.NewBatch(int64(len(txs)))
	// iterate from 0 to allTxCount to index all txs
	for i, txResult := range txs {
		txResult.Index = uint32(i)
		if err := batch.Add(txResult); err != nil {
			return fmt.Errorf("failed to add txs to txstore due to: %w", err)
		}
	}
	if err := t.idx.AddBatch(batch); err != nil {
		return fmt.Errorf("failed to add txs to txstore: %w", err)
	}

	return nil
}

func (t *txstore) rollbackOneBlock(batch dbm.Batch, height int64) error {
	// This is a bit hacky but it's the only way we have to remove txs from the indexer.
	// The indexer stores txs in its underlying DB by constructing a key that uses a combination
	// of height, indexes and hardcoded labels. For more details, see how the keys are constructed here:
	//    https://github.com/cometbft/cometbft/blob/v0.37.2/state/txindex/kv/kv.go#L640
	// The iterator below is a cheap trick to iterarte through all transactions that belong to a block
	// at height 'height', that is done with the 'start' of the iterator
	// The 'end' part is where the magic happens: we construct a key that ends in a character with a higher ASCII
	// value than any other just so we can include all keys "until the end"
	it, err := t.db.Iterator(
		[]byte(fmt.Sprintf("%s/%d/%d/", types.TxHeightKey, height, height)),
		[]byte(fmt.Sprintf("%s/%d/%d~", types.TxHeightKey, height, height)),
	)
	defer it.Close()
	if err != nil {
		return err
	}

	// the indexer stores data with a primary and secondary key:
	// primary:   height-based-key -> tx_hash
	// secondary: tx_hash          -> tx_bytes
	// both primary and secondary keys are the corresponding key/value from the iterator.
	for ; it.Valid(); it.Next() {
		if err := batch.Delete(it.Key()); err != nil {
			return err
		}
		if err := batch.Delete(it.Value()); err != nil {
			return err
		}
	}
	return nil
}

// Access the underlying db used by the txindexer and removes all transactions. It needs
// both the starting point (rollbackHeight) as well as the latest height since there's no other
// way of knowing what's the last tx in the indexer otherwise.
func (t *txstore) RollbackToHeight(rollbackHeight, currentHeight int64) error {
	batch := t.db.NewBatch()
	defer batch.Close()

	for h := rollbackHeight + 1; h <= currentHeight; h++ {
		if err := t.rollbackOneBlock(batch, h); err != nil {
			return err
		}
	}
	if err := batch.WriteSync(); err != nil {
		panic(err)
	}
	return nil
}
