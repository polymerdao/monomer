package localdb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/cockroachdb/pebble"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/monomerdb"
)

var (
	endian            = binary.BigEndian
	unsafeLabelKey    = bucketHashByLabel.Key([]byte(eth.Unsafe))
	safeLabelKey      = bucketHashByLabel.Key([]byte(eth.Safe))
	finalizedLabelKey = bucketHashByLabel.Key([]byte(eth.Finalized))
	heightKey         = bucketHeight.Key()
)

type DB struct {
	db *pebble.DB
}

func New(db *pebble.DB) *DB {
	return &DB{
		db: db,
	}
}

// TODO: optimization - we can use a cbor encoder to write directly to a batch, rather than copying bytes twice.

// AppendBlock does no validity checks and does not update labels.
func (db *DB) AppendBlock(block *monomer.Block) error {
	headerBytes, err := cbor.Marshal(block.Header)
	if err != nil {
		return fmt.Errorf("marshal header into cbor: %v", err)
	}
	heightBytes := marshalUint64(uint64(block.Header.Height))
	return db.update(func(b *pebble.Batch) error {
		if err := b.Set(bucketHeaderByHeight.Key(heightBytes), headerBytes, nil); err != nil {
			return fmt.Errorf("set block by height: %v", err)
		}
		if err := b.Set(bucketHeightByHash.Key(block.Header.Hash.Bytes()), heightBytes, nil); err != nil {
			return fmt.Errorf("set height by hash: %v", err)
		}
		if err := b.Set(heightKey, heightBytes, nil); err != nil {
			return fmt.Errorf("set height: %v", err)
		}

		for i, tx := range block.Txs {
			heightAndIndexBytes := slices.Concat(heightBytes, marshalUint64(uint64(i)))
			if err := b.Set(bucketTxByHeightAndIndex.Key(heightAndIndexBytes), tx, nil); err != nil {
				return fmt.Errorf("set tx by height and index: %v", err)
			}
			if err := b.Set(bucketTxHeightAndIndexByHash.Key(tx.Hash()), heightAndIndexBytes, nil); err != nil {
				return fmt.Errorf("set tx height and index by hash: %v", err)
			}
		}
		return nil
	})
}

func (db *DB) UpdateLabels(unsafe, safe, finalized common.Hash) error {
	return db.update(func(b *pebble.Batch) error {
		return updateLabels(b, unsafe, safe, finalized)
	})
}

func updateLabels(b *pebble.Batch, unsafe, safe, finalized common.Hash) error {
	if err := b.Set(unsafeLabelKey, unsafe.Bytes(), nil); err != nil {
		return fmt.Errorf("set unsafe hash by label: %v", err)
	}
	if err := b.Set(safeLabelKey, safe.Bytes(), nil); err != nil {
		return fmt.Errorf("set safe hash by label: %v", err)
	}
	if err := b.Set(finalizedLabelKey, finalized.Bytes(), nil); err != nil {
		return fmt.Errorf("set finalized hash by label: %v", err)
	}
	return nil
}

// Rollback rolls back the chain and updates labels.
func (db *DB) Rollback(unsafe, safe, finalized common.Hash) error {
	return db.updateIndexed(func(b *pebble.Batch) (err error) {
		unsafeHeightBytesValue, closer, err := get(b, bucketHeightByHash.Key(unsafe.Bytes()))
		if err != nil {
			return fmt.Errorf("get height by hash %s: %w", unsafe, err)
		}
		defer func() {
			err = wrapCloseErr(err, closer)
		}()
		unsafeHeight := endian.Uint64(unsafeHeightBytesValue)
		firstHeightBytesToDelete := marshalUint64(unsafeHeight + 1)

		// Set the new height.
		if err := b.Set(heightKey, unsafeHeightBytesValue, nil); err != nil {
			return fmt.Errorf("set height: %v", err)
		}

		// Delete headers.
		firstHeaderToDelete := bucketHeaderByHeight.Key(firstHeightBytesToDelete)
		nextBucket := (bucketHeaderByHeight + 1).Key()
		headerIter, err := b.NewIter(&pebble.IterOptions{
			LowerBound: firstHeaderToDelete,
			UpperBound: nextBucket,
		})
		if err != nil {
			return fmt.Errorf("new bucketHeaderByHeight iterator: %v", err)
		}
		defer func() {
			err = wrapCloseErr(err, headerIter)
		}()
		header := new(monomer.Header)
		for headerIter.First(); headerIter.Valid(); headerIter.Next() {
			value, err := headerIter.ValueAndErr()
			if err != nil {
				return fmt.Errorf("get header value from iterator: %v", err)
			}
			if err := cbor.Unmarshal(value, &header); err != nil {
				return fmt.Errorf("unmarshal header from cbor: %v", err)
			}
			if err := b.Delete(bucketHeightByHash.Key(header.Hash.Bytes()), nil); err != nil {
				return fmt.Errorf("delete height by hash %s: %v", header.Hash, err)
			}
		}
		if err := b.DeleteRange(firstHeaderToDelete, nextBucket, nil); err != nil {
			return fmt.Errorf("delete range of headers: %v", err)
		}

		// Delete transactions.
		firstTxToDelete := bucketTxByHeightAndIndex.Key(firstHeightBytesToDelete)
		nextBucket = (bucketTxByHeightAndIndex + 1).Key()
		txIter, err := b.NewIter(&pebble.IterOptions{
			LowerBound: firstTxToDelete,
			UpperBound: nextBucket,
		})
		if err != nil {
			return fmt.Errorf("new bucketTxByHeightAndIndex iterator: %v", err)
		}
		defer func() {
			err = wrapCloseErr(err, txIter)
		}()
		for txIter.First(); txIter.Valid(); txIter.Next() {
			value, err := txIter.ValueAndErr()
			if err != nil {
				return fmt.Errorf("get tx value from iterator: %v", err)
			}
			hash := bfttypes.Tx(value).Hash()
			if err := b.Delete(bucketTxHeightAndIndexByHash.Key(hash), nil); err != nil {
				return fmt.Errorf("delete tx height and index by hash %v: %v", hash, err)
			}
		}
		if err := b.DeleteRange(
			bucketTxHeightAndIndexByHash.Key(firstHeightBytesToDelete),
			marshalUint64(uint64(bucketTxHeightAndIndexByHash)+1),
			nil,
		); err != nil {
			return fmt.Errorf("delete range of txs: %v", err)
		}

		if err := updateLabels(b, unsafe, safe, finalized); err != nil {
			return fmt.Errorf("update labels: %v", err)
		}
		return nil
	})
}

func (db *DB) HeadHeader() (*monomer.Header, error) {
	var header *monomer.Header
	if err := db.view(func(s *pebble.Snapshot) error {
		heightBytes, err := getHeight(s)
		if err != nil {
			return fmt.Errorf("get height: %w", err)
		}
		header, err = headerByHeight(s, heightBytes)
		if err != nil {
			return fmt.Errorf("get header by height: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return header, nil
}

func (db *DB) HeadBlock() (*monomer.Block, error) {
	var block *monomer.Block
	if err := db.view(func(s *pebble.Snapshot) error {
		heightBytes, err := getHeight(s)
		if err != nil {
			return fmt.Errorf("get height: %w", err)
		}
		block, err = blockByHeight(s, heightBytes)
		if err != nil {
			return fmt.Errorf("get block by height: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return block, nil
}

func getHeight(s *pebble.Snapshot) ([]byte, error) {
	heightBytesValue, closer, err := get(s, heightKey)
	if err != nil {
		return nil, err
	}
	heightBytes := make([]byte, len(heightBytesValue))
	copy(heightBytes, heightBytesValue)
	if err := closer.Close(); err != nil {
		return nil, fmt.Errorf("close: %v", err)
	}
	return heightBytes, nil
}

func (db *DB) Height() (uint64, error) {
	heightBytesValue, closer, err := get(db.db, heightKey)
	if err != nil {
		return 0, err
	}
	height := endian.Uint64(heightBytesValue)
	if err := closer.Close(); err != nil {
		return 0, fmt.Errorf("close: %v", err)
	}
	return height, nil
}

func (db *DB) BlockByHeight(height uint64) (*monomer.Block, error) {
	var block *monomer.Block
	if err := db.view(func(s *pebble.Snapshot) error {
		var err error
		block, err = blockByHeight(s, marshalUint64(height))
		return err
	}); err != nil {
		return nil, err
	}
	return block, nil
}

func blockByHeight(s *pebble.Snapshot, heightBytes []byte) (*monomer.Block, error) {
	header, err := headerByHeight(s, heightBytes)
	if err != nil {
		return nil, fmt.Errorf("get header by height: %w", err)
	}
	txs, err := txsInRange(s, heightBytes, marshalUint64(endian.Uint64(heightBytes)+1))
	if err != nil {
		return nil, err
	}
	return monomer.NewBlock(header, txs), nil
}

func (db *DB) BlockByHash(hash common.Hash) (*monomer.Block, error) {
	var header *monomer.Header
	var txs bfttypes.Txs
	if err := db.view(func(s *pebble.Snapshot) (err error) {
		header, err = headerByHash(s, hash)
		if err != nil {
			return fmt.Errorf("get header by hash: %w", err)
		}
		txs, err = txsInRange(s, marshalUint64(uint64(header.Height)), marshalUint64(uint64(header.Height+1)))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return monomer.NewBlock(header, txs), nil
}

func (db *DB) BlockByLabel(label eth.BlockLabel) (*monomer.Block, error) {
	var header *monomer.Header
	var txs bfttypes.Txs
	if err := db.view(func(s *pebble.Snapshot) error {
		var err error
		header, err = headerByLabel(s, label)
		if err != nil {
			return fmt.Errorf("get header by label: %w", err)
		}
		txs, err = txsInRange(s, marshalUint64(uint64(header.Height)), marshalUint64(uint64(header.Height+1)))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return monomer.NewBlock(header, txs), nil
}

func (db *DB) HeaderByHash(hash common.Hash) (*monomer.Header, error) {
	var header *monomer.Header
	if err := db.view(func(s *pebble.Snapshot) error {
		var err error
		header, err = headerByHash(s, hash)
		return err
	}); err != nil {
		return nil, err
	}
	return header, nil
}

func (db *DB) HeaderByLabel(label eth.BlockLabel) (*monomer.Header, error) {
	var header *monomer.Header
	if err := db.view(func(s *pebble.Snapshot) error {
		var err error
		header, err = headerByLabel(s, label)
		return err
	}); err != nil {
		return nil, err
	}
	return header, nil
}

func (db *DB) HeaderByHeight(height uint64) (*monomer.Header, error) {
	return headerByHeight(db.db, marshalUint64(height))
}

func txsInRange(s *pebble.Snapshot, startHeightBytes, endHeightBytes []byte) (_ bfttypes.Txs, err error) {
	iter, err := s.NewIter(&pebble.IterOptions{
		LowerBound: bucketTxByHeightAndIndex.Key(startHeightBytes),
		UpperBound: bucketTxByHeightAndIndex.Key(endHeightBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("new iterator: %v", err)
	}
	defer func() {
		err = wrapCloseErr(err, iter)
	}()
	txs := bfttypes.Txs{}
	for iter.First(); iter.Valid(); iter.Next() {
		value, err := iter.ValueAndErr()
		if err != nil {
			return nil, fmt.Errorf("get value from iterator: %v", err)
		}
		tx := make([]byte, len(value))
		copy(tx, value)
		txs = append(txs, tx)
	}
	return txs, nil
}

type getter interface {
	Get([]byte) ([]byte, io.Closer, error)
}

func headerByHeight(g getter, heightBytes []byte) (_ *monomer.Header, err error) {
	headerBytes, closer, err := get(g, bucketHeaderByHeight.Key(heightBytes))
	if err != nil {
		return nil, err
	}
	defer func() {
		err = wrapCloseErr(err, closer)
	}()

	h := new(monomer.Header)
	if err := cbor.Unmarshal(headerBytes, &h); err != nil {
		return nil, fmt.Errorf("unmarshal cbor: %v", err)
	}
	return h, nil
}

func headerByLabel(s *pebble.Snapshot, label eth.BlockLabel) (_ *monomer.Header, err error) {
	hashBytes, closer, err := get(s, bucketHashByLabel.Key([]byte(label)))
	if err != nil {
		return nil, fmt.Errorf("get label hash: %w", err)
	}
	defer func() {
		err = wrapCloseErr(err, closer)
	}()
	header, err := headerByHash(s, common.Hash(hashBytes))
	if err != nil {
		return nil, fmt.Errorf("header by hash: %w", err)
	}
	return header, nil
}

func headerByHash(s *pebble.Snapshot, hash common.Hash) (_ *monomer.Header, err error) {
	heightBytes, closer, err := get(s, bucketHeightByHash.Key(hash.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("get height by hash: %w", err)
	}
	defer func() {
		err = wrapCloseErr(err, closer)
	}()
	header, err := headerByHeight(s, heightBytes)
	if err != nil {
		return nil, fmt.Errorf("get header by height: %w", err)
	}
	return header, nil
}

func (db *DB) view(cb func(*pebble.Snapshot) error) (err error) {
	s := db.db.NewSnapshot()
	defer func() {
		err = wrapCloseErr(err, s)
	}()
	return cb(s)
}

func (db *DB) updateIndexed(cb func(*pebble.Batch) error) (err error) {
	b := db.db.NewIndexedBatch()
	defer func() {
		err = wrapCloseErr(err, b)
	}()
	if err := cb(b); err != nil {
		return err
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("commit: %v", err)
	}
	return nil
}

func (db *DB) update(cb func(*pebble.Batch) error) (err error) {
	b := db.db.NewBatch()
	defer func() {
		err = wrapCloseErr(err, b)
	}()
	if err := cb(b); err != nil {
		return err
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("commit: %v", err)
	}
	return nil
}

func get(g getter, key []byte) (_ []byte, _ io.Closer, err error) {
	value, closer, err := g.Get(key)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, nil, monomerdb.ErrNotFound
		}
		return nil, nil, fmt.Errorf("%v", err) // Obfuscate the error.
	}
	return value, closer, nil
}

func marshalUint64(x uint64) []byte {
	bytes := make([]byte, 8) //nolint:gomnd
	endian.PutUint64(bytes, x)
	return bytes
}

func wrapCloseErr(err error, closer io.Closer) error {
	closeErr := closer.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close: %v", closeErr)
	}
	if err != nil || closeErr != nil {
		return multierror.Append(err, closeErr)
	}
	return nil
}
