package payloadstore

import (
	"sync"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/polymerdao/monomer"
)

type PayloadStore interface {
	Add(payload *monomer.Payload)
	Get(id engine.PayloadID) (*monomer.Payload, bool)
	Current() *monomer.Payload
	RollbackToHeight(height int64) error
}

type pstore struct {
	mutex    sync.Mutex
	payloads map[engine.PayloadID]*monomer.Payload
	heights  map[int64]engine.PayloadID
	current  *monomer.Payload
}

var _ PayloadStore = (*pstore)(nil)

func NewPayloadStore() PayloadStore {
	return &pstore{
		mutex:    sync.Mutex{},
		payloads: make(map[engine.PayloadID]*monomer.Payload),
		heights:  make(map[int64]engine.PayloadID),
	}
}

func (p *pstore) Add(payload *monomer.Payload) {
	id := payload.ID()
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if _, ok := p.payloads[*id]; !ok {
		p.heights[payload.Height] = *id
		p.payloads[*id] = payload
		p.current = payload
	}
}

func (p *pstore) Get(id engine.PayloadID) (*monomer.Payload, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if payload, ok := p.payloads[id]; ok {
		return payload, true
	}
	return nil, false
}

func (p *pstore) Current() *monomer.Payload {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.current
}

func (p *pstore) RollbackToHeight(height int64) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// nuke everything in memory
	p.current = nil
	p.heights = make(map[int64]engine.PayloadID)
	p.payloads = make(map[engine.PayloadID]*monomer.Payload)

	return nil
}
