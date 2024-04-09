package payloadstore

import (
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/polymerdao/monomer"
)

type Store struct {
	payloads map[engine.PayloadID]*monomer.Payload
	heights  map[int64]engine.PayloadID
	current  *monomer.Payload
}

func NewPayloadStore() *Store {
	return &Store{
		payloads: make(map[engine.PayloadID]*monomer.Payload),
		heights:  make(map[int64]engine.PayloadID),
	}
}

func (p *Store) Add(payload *monomer.Payload) {
	id := payload.ID()
	if _, ok := p.payloads[*id]; !ok {
		p.heights[payload.Height] = *id
		p.payloads[*id] = payload
		p.current = payload
	}
}

func (p *Store) Get(id engine.PayloadID) (*monomer.Payload, bool) {
	payload, ok := p.payloads[id]
	return payload, ok
}

func (p *Store) Current() *monomer.Payload {
	return p.current
}

func (p *Store) RollbackToHeight(height int64) error {
	// nuke everything in memory
	p.current = nil
	p.heights = make(map[int64]engine.PayloadID)
	p.payloads = make(map[engine.PayloadID]*monomer.Payload)

	return nil
}
