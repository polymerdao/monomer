package payloadstore

import (
	"fmt"

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

// RollbackToHeight removes all payloads at heights greater than the given height
// and sets the current payload to the highest height payload that remains.
//
// If there is no payload at the given height, it still performs the pruning,
// but returns an error as an indication.
func (p *Store) RollbackToHeight(height int64) error {
	max := int64(0)

	for h, id := range p.heights {
		if h > height {
			delete(p.payloads, id)
			delete(p.heights, h)
		} else if h > max {
			max = h
		}
	}

	// set the current payload to the highest height
	p.current = p.payloads[p.heights[max]]

	if _, exists := p.heights[height]; !exists {
		return fmt.Errorf("no existing payload at height %d", height)
	}

	return nil
}

// Clear removes all payloads from the store.
func (p *Store) Clear() {
	// nuke everything in memory
	p.current = nil
	p.heights = make(map[int64]engine.PayloadID)
	p.payloads = make(map[engine.PayloadID]*monomer.Payload)
}
