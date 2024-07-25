package eth

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer"
)

type BlockID struct {
	Label  eth.BlockLabel
	Height int64
}

func (id *BlockID) UnmarshalJSON(data []byte) error {
	var dataStr string
	if err := json.Unmarshal(data, &dataStr); err != nil {
		return fmt.Errorf("unmarshal block id into string: %v", err)
	}

	switch dataStr {
	case eth.Unsafe, eth.Safe, eth.Finalized:
		id.Label = eth.BlockLabel(dataStr)
	default:
		var height hexutil.Uint64
		if err := height.UnmarshalText([]byte(dataStr)); err != nil {
			return fmt.Errorf("unmarshal height as hexutil.Uint64: %v", err)
		}
		id.Height = int64(height)
	}
	return nil
}

type BlockIDDatabase interface {
	BlockByLabel(eth.BlockLabel) (*monomer.Block, error)
	BlockByHeight(uint64) (*monomer.Block, error)
}

func (id *BlockID) Get(s BlockIDDatabase) (*monomer.Block, error) {
	if id.Label != "" {
		return s.BlockByLabel(id.Label)
	}
	return s.BlockByHeight(uint64(id.Height))
}
