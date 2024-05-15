package e2e

import (
	"github.com/polymerdao/monomer/node"
	"golang.org/x/exp/slog"
)

// NodeSelectiveListener aliases node.SelectiveListener to avoid name collisions.
type NodeSelectiveListener = node.SelectiveListener

type SelectiveListener struct {
	*NodeSelectiveListener

	OPLogWithPrefixCb func(prefix string, r *slog.Record)
	OnAnvilErrCb      func(error)
}

func (s *SelectiveListener) LogWithPrefix(prefix string, r *slog.Record) {
	if s.OPLogWithPrefixCb != nil {
		s.OPLogWithPrefixCb(prefix, r)
	}
}

func (s *SelectiveListener) OnAnvilErr(err error) {
	if s.OnAnvilErrCb != nil {
		s.OnAnvilErrCb(err)
	}
}
