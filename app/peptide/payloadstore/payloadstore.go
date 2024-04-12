package payloadstore

import (
	"fmt"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/polymerdao/monomer"
)

// Store is an in-memory store for payloads.
// It is not thread-safe and should be used with a single goroutine.
type Store struct {
	payloads map[engine.PayloadID]*monomer.Payload
	heights  map[int64]engine.PayloadID
	current  *monomer.Payload
}

func New() *Store {
	return &Store{
		payloads: make(map[engine.PayloadID]*monomer.Payload),
		heights:  make(map[int64]engine.PayloadID),
	}
}

// Add adds a payload to the store and updates the current payload
// if the new payload has a greater height.
func (p *Store) Add(payload *monomer.Payload) {
	id := payload.ID()
	if _, ok := p.payloads[*id]; !ok {
		p.heights[payload.Height] = *id
		p.payloads[*id] = payload

		if (p.current == nil) || (payload.Height > p.current.Height) {
			p.current = payload
		}
	}
}

// Remove removes a payload from the store and updates the current payload
// if necessary.
func (p *Store) Remove(id engine.PayloadID) {
	current := p.Current()
	resetCurrent := id == *current.ID()

	if payload, ok := p.payloads[id]; ok {
		delete(p.payloads, id)
		delete(p.heights, payload.Height)
	}

	if resetCurrent {
		p.current = nil
		for _, payload := range p.payloads {
			if p.current == nil || payload.Height > p.current.Height {
				p.current = payload
			}
		}
	}
}

func (p *Store) Get(id engine.PayloadID) *monomer.Payload {
	return p.payloads[id]
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
