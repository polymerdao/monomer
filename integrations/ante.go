package integrations

import (
	"sync"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

var lastBlock int64
var lastBlockMu sync.Mutex

func WrapAnteHandler(ante sdktypes.AnteHandler) sdktypes.AnteHandler {
	return func(ctx sdktypes.Context, tx sdktypes.Tx, simulate bool) (newCtx sdktypes.Context, err error) {
		lastBlockMu.Lock()
		if lastBlock < ctx.BlockHeight() && lastBlock != 1 {
			// Handle deposits (first tx in every block except the genesis block)
			lastBlock++
			return ctx, nil
		}
		lastBlockMu.Unlock()
		return ante(ctx, tx, simulate)
	}
}
