package types

import (
	"math/big"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/common"
)

// NewGenesisConfig creates a new Genesis from l1 origin's hash/number; L2 genesis will be filled by L2 chain when it mines
// the genesis block
func NewGenesisConfig(l1Hash Hash, l1Number uint64) *rollup.Genesis {
	return &rollup.Genesis{L1: eth.BlockID{Hash: l1Hash, Number: l1Number}, L2: eth.BlockID{}}
}

// NewBlockInfo returns a BlockInfo interface implemented by a MockBlockInfo
func NewBlockInfo(hash, parentHeash Hash, number, time uint64) eth.BlockInfo {
	return &testutils.MockBlockInfo{
		InfoHash:       hash,
		InfoParentHash: parentHeash,
		InfoNum:        number,
		InfoTime:       time,
		InfoCoinbase:   common.BytesToAddress([]byte("polymer")),
		InfoBaseFee:    big.NewInt(0),
	}
}
