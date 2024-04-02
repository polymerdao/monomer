package mempool

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	dbm "github.com/cometbft/cometbft-db"
	comettypes "github.com/cometbft/cometbft/types"
)

const (
	poolLengthKey = "poolLength"
	headKey       = "headKey"
	tailKey       = "tailKey"
)

// Pool stores the transactions in a linked list for its inherent FCFS behaviour
type storageElem struct {
	Txn      comettypes.Tx `json:"txn"`
	NextHash []byte        `json:"nextHash"`
}

type Pool struct {
	db dbm.DB
}

func New(db dbm.DB) *Pool {
	return &Pool{
		db: db,
	}
}

func (p *Pool) Enqueue(userTxn comettypes.Tx) error {
	// NOTE: we should do reads and writes on the same view. Right now they occur on separate views.
	// Unfortunately, comet's DB interface doesn't support it.
	// Moving to a different DB interface is left for future work.

	batch := p.db.NewBatch()
	defer batch.Close() // TODO: catch error.

	tail, err := p.db.Get([]byte(tailKey))
	if err != nil {
		return err
	}

	if err = p.putElem(batch, userTxn.Hash(), &storageElem{
		Txn: userTxn,
	}); err != nil {
		return nil
	}

	if tail != nil {
		var oldTail *storageElem
		oldTail, err = p.elem(tail)
		if err != nil {
			return fmt.Errorf("get tail: %v", err)
		}

		// update old tail to point to the new item
		oldTail.NextHash = userTxn.Hash()
		if err = p.putElem(batch, tail, oldTail); err != nil {
			return err
		}
	} else {
		// empty list, make new item both the head and the tail
		if err = batch.Set([]byte(headKey), userTxn.Hash()); err != nil {
			return err
		}
	}

	if err = batch.Set([]byte(tailKey), userTxn.Hash()); err != nil {
		return err
	}

	pLen, err := p.Len()
	if err != nil {
		return err
	}

	if err := p.updateLen(batch, pLen+1); err != nil { // don't worry about overflows, highly unlikely
		return err
	}

	return batch.WriteSync()
}

// Dequeue returns the transaction with the highest priority from the pool
func (p *Pool) Dequeue() (comettypes.Tx, error) {
	pLen, err := p.Len()
	if err != nil {
		return nil, err
	}

	headHash, err := p.db.Get([]byte(headKey))
	if err != nil {
		return nil, err
	} else if headHash == nil {
		return nil, errors.New("head hash not found")
	}

	headElem, err := p.elem(headHash)
	if err != nil {
		return nil, fmt.Errorf("get head element: %v", err)
	} else if headElem == nil {
		return nil, errors.New("head elem not found")
	}

	batch := p.db.NewBatch()
	defer batch.Close() // TODO catch error.

	if err = batch.Delete(headHash); err != nil {
		return nil, err
	}

	if headElem.NextHash == nil {
		// the list is empty now
		if err = batch.Delete([]byte(headKey)); err != nil {
			return nil, err
		}
		if err = batch.Delete([]byte(tailKey)); err != nil {
			return nil, err
		}
	} else {
		if err = batch.Set([]byte(headKey), headElem.NextHash); err != nil {
			return nil, err
		}
	}

	if err = p.updateLen(batch, pLen-1); err != nil {
		return nil, err
	}

	if err := batch.WriteSync(); err != nil {
		return nil, err
	}

	return headElem.Txn, nil
}

func (p *Pool) Len() (uint64, error) {
	lengthBytes, err := p.db.Get([]byte(poolLengthKey))
	if err != nil {
		return 0, nil
	}
	if lengthBytes == nil {
		return 0, nil
	}
	return binary.BigEndian.Uint64(lengthBytes), nil
}

func (p *Pool) updateLen(batch dbm.Batch, l uint64) error {
	return batch.Set([]byte(poolLengthKey), binary.BigEndian.AppendUint64(nil, l))
}

func (p *Pool) elem(itemKey []byte) (*storageElem, error) {
	value, err := p.db.Get(itemKey)
	if err != nil {
		return nil, err
	} else if value == nil {
		return nil, errors.New("value not found")
	}
	item := new(storageElem)
	if err := json.Unmarshal(value, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (p *Pool) putElem(batch dbm.Batch, itemKey []byte, item *storageElem) error {
	itemBytes, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return batch.Set(itemKey, itemBytes)
}
