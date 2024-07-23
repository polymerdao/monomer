package ethapi

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	types "github.com/ethereum/go-ethereum/core/types"
)

type Backend interface {
	GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error)
}
