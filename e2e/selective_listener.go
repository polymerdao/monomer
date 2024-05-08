package e2e

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/polymerdao/monomer/node"
)

// NodeSelectiveListener aliases node.SelectiveListener to avoid name collisions.
type NodeSelectiveListener = node.SelectiveListener

type SelectiveListener struct {
	*NodeSelectiveListener

	OPLogWithPrefixCb func(prefix string, r *log.Record)
	OnAnvilErrCb      func(error)
}

func (s *SelectiveListener) LogWithPrefix(prefix string, r *log.Record) {
	if s.OPLogWithPrefixCb != nil {
		s.OPLogWithPrefixCb(prefix, r)
	}
}

func (s *SelectiveListener) OnAnvilErr(err error) {
	if s.OnAnvilErrCb != nil {
		s.OnAnvilErrCb(err)
	}
}
