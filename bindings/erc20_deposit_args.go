package bindings

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type RelayMessageArgs struct {
	Nonce       *big.Int
	Sender      common.Address
	Target      common.Address
	Value       *big.Int
	MinGasLimit *big.Int
	Message     []byte
}

type FinalizeBridgeERC20Args struct {
	RemoteToken common.Address
	LocalToken  common.Address
	From        common.Address
	To          common.Address
	Amount      *big.Int
	ExtraData   []byte
}
