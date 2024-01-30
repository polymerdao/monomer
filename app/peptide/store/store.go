package store

import (
	"fmt"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type BlockStore interface {
	// Adds a new block to the store
	//
	// NOTE: no block by label is updated. We have to call UpdateLabel to update labels explicitly including "unsafe"
	// label.
	// EE client won't observe latest block changes until after Forkchoice is performed on the added block
	AddBlock(block eetypes.BlockData)

	// Given a block already in the store it updates its label to `label`. A common use case for this method
	// is when the op-node informs the engine of a new finalised block so the engine can go back to the
	// store and update it.
	// Returns error in case the block is not found
	UpdateLabel(label eth.BlockLabel, hash eetypes.Hash) error

	// Retrieves a block from the store by its hash and returns it. It uses the user-provided BlockUnmarshaler
	// callback to do unmarshal the opaque bytes into an actual block. Returns the block if found or nil otherwise.
	BlockByHash(hash eetypes.Hash) eetypes.BlockData

	// Retrieves a block from the store by its height and returns it. It uses the user-provided BlockUnmarshaler
	// callback to do unmarshal the opaque bytes into an actual block. Returns the block if found or nil otherwise.
	BlockByNumber(height int64) eetypes.BlockData

	// Retrieves a block from the store by its label and returns it. It uses the user-provided BlockUnmarshaler
	// callback to do unmarshal the opaque bytes into an actual block. Returns the block if found or nil otherwise.
	BlockByLabel(label eth.BlockLabel) eetypes.BlockData

	// Removes all blocks up to (not including) height. Note that updating any label that may go stale
	// is up to the caller
	RollbackToHeight(height int64) error

	// TODO might need a new method to handle re-org cases. we could do it when the finalised label is applied
	//      but having an explicit method for it could be better?
}

// We hide the block marshaling behind this function so the store does not need to know
// about any concrete type when fetching blocks from the db. The unmarshaling is up to
// the caller.
type BlockUnmarshaler func(bz []byte) (eetypes.BlockData, error)

/*
* BlockStore implementation
* Main key: {block hash => block bytes}
* Secondary keys:
*   - {height => block hash}
*   - {label => block hash}
 */
type blockStore struct {
	// we trust the db handles concurrency accordingly.
	// For now, we don't need mutexes
	db           dbm.DB
	unmarshaller BlockUnmarshaler

	// TODO store pointers to blocks in a list so whenever there's a re-org we can walk the tree up
	//      until the new root and prune everything in between
}

var _ BlockStore = (*blockStore)(nil)

func NewBlockStore(db dbm.DB, unmarshaller BlockUnmarshaler) BlockStore {
	return &blockStore{
		db:           db,
		unmarshaller: unmarshaller,
	}
}

func (b *blockStore) AddBlock(block eetypes.BlockData) {
	// use batching for atomic updates
	batch := b.db.NewBatch()
	defer batch.Close()

	hash := block.Hash()
	if err := batch.Set(hashKey(hash), block.Bytes()); err != nil {
		panic(err)
	}
	if err := batch.Set(heightKey(block.Height()), hash[:]); err != nil {
		panic(err)
	}
	if err := batch.WriteSync(); err != nil {
		panic(err)
	}
}

func (b *blockStore) UpdateLabel(label eth.BlockLabel, hash eetypes.Hash) error {
	found, err := b.db.Has(hashKey(hash))
	if err != nil {
		panic(err)
	}
	if !found {
		return fmt.Errorf("block not found hash: %x", hash)
	}
	if err := b.db.SetSync(labelKey(label), hash[:]); err != nil {
		panic(err)
	}
	return nil
}

func (b *blockStore) BlockByHash(hash eetypes.Hash) eetypes.BlockData {
	bz := b.get(hashKey(hash))
	if len(bz) == 0 {
		return nil
	}
	block, err := b.unmarshaller(bz)
	if err != nil {
		return nil
	}
	return block
}

func (b *blockStore) BlockByNumber(height int64) eetypes.BlockData {
	bz := b.get(heightKey(height))
	if len(bz) == 0 {
		return nil
	}
	var hash eetypes.Hash
	copy(hash[:], bz)
	return b.BlockByHash(hash)
}

func (b *blockStore) BlockByLabel(label eth.BlockLabel) eetypes.BlockData {
	bz := b.get(labelKey(label))
	if len(bz) == 0 {
		return nil
	}
	var hash eetypes.Hash
	copy(hash[:], bz)
	return b.BlockByHash(hash)
}

// RollbackToHeight removes all blocks with blockHeight > rollbackHeight
// Both hash and height keys are deleted
// PeptideNode will call blockStore.UpdateLabel to update 3 head labels
// TODO: perhaps make BlockStore rollback atomic?
func (b *blockStore) RollbackToHeight(height int64) error {
	batch := b.db.NewBatch()
	defer batch.Close()

	for h := height + 1; ; h++ {
		secondaryKey := heightKey(h)
		hash := b.get(secondaryKey)
		if len(hash) == 0 {
			break
		}

		// if an error occurs here, the whole batch is discarded and
		// no changes to the db are written
		if err := batch.Delete(secondaryKey); err != nil {
			return err
		}
		mainKey := hashKey(eetypes.Hash(hash))
		if err := batch.Delete(mainKey); err != nil {
			return err
		}
	}
	if err := batch.WriteSync(); err != nil {
		panic(err)
	}
	return nil
}

func (b *blockStore) get(key []byte) []byte {
	bz, err := b.db.Get(key)
	if err != nil {
		panic(err)
	}
	return bz
}

func hashKey(hash eetypes.Hash) []byte {
	return []byte(fmt.Sprintf("bh:%x", hash))
}

func heightKey(height int64) []byte {
	return []byte(fmt.Sprintf("h:%x", height))
}

func labelKey(label eth.BlockLabel) []byte {
	return []byte(fmt.Sprintf("l:%s", label))
}
