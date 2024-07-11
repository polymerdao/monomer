package e2e

import (
	"github.com/polymerdao/monomer/node"
	"golang.org/x/exp/slog"
)

// NodeSelectiveListener aliases node.SelectiveListener to avoid name collisions.
type NodeSelectiveListener = node.SelectiveListener

type SelectiveListener struct {
	*NodeSelectiveListener

	OPLogCb func(slog.Record)
}

func (s *SelectiveListener) Log(r slog.Record) { //nolint:gocritic // hugeParam
	if s.OPLogCb != nil {
		s.OPLogCb(r)
	}
}
