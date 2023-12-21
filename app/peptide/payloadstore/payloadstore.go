package payloadstore

import (
	"fmt"
	"sync"

	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type PayloadStore interface {
	Add(payload *eetypes.Payload) error
	Get(id eetypes.PayloadID) (*eetypes.Payload, bool)
	Current() *eetypes.Payload
	RollbackToHeight(height int64) error
}

type pstore struct {
	mutex    sync.Mutex
	payloads map[eetypes.PayloadID]*eetypes.Payload
	heights  map[int64]eetypes.PayloadID
	current  *eetypes.Payload
}

var _ PayloadStore = (*pstore)(nil)

func NewPayloadStore() PayloadStore {
	return &pstore{
		mutex:    sync.Mutex{},
		payloads: make(map[eetypes.PayloadID]*eetypes.Payload),
		heights:  make(map[int64]eetypes.PayloadID),
	}
}

func (p *pstore) Add(payload *eetypes.Payload) error {
	if payload == nil {
		return fmt.Errorf("could not add invalid payload")
	}
	id, err := payload.GetPayloadID()
	if err != nil {
		return fmt.Errorf("could not add payload, %w", err)
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if _, ok := p.payloads[*id]; !ok {
		p.heights[payload.Height] = *id
		p.payloads[*id] = payload
		p.current = payload
	}
	return nil
}

func (p *pstore) Get(id eetypes.PayloadID) (*eetypes.Payload, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if payload, ok := p.payloads[id]; ok {
		return payload, true
	}
	return nil, false
}

func (p *pstore) Current() *eetypes.Payload {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.current
}

func (p *pstore) RollbackToHeight(height int64) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if id, ok := p.heights[height]; !ok {
		return fmt.Errorf("invalid height %v", height)
	} else if current, ok := p.payloads[id]; !ok {
		panic("payload store corrupted")
	} else {
		p.current = current
	}

	for i := height + 1; ; i++ {
		id, ok := p.heights[i]
		if !ok {
			break
		}
		delete(p.heights, i)
		delete(p.payloads, id)
	}
	return nil
}
