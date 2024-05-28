package e2e

import (
	"io"

	"github.com/polymerdao/monomer/node"
	"golang.org/x/exp/slog"
)

// NodeSelectiveListener aliases node.SelectiveListener to avoid name collisions.
type NodeSelectiveListener = node.SelectiveListener

type SelectiveListener struct {
	*NodeSelectiveListener

	OPLogCb func(slog.Record)

	HandleCmdOutputCb func(path string, stdout, stderr io.Reader)
	OnAnvilErrCb      func(error)
}

func (s *SelectiveListener) HandleCmdOutput(path string, stdout, stderr io.Reader) {
	if s.HandleCmdOutputCb != nil {
		s.HandleCmdOutputCb(path, stdout, stderr)
	}
}

func (s *SelectiveListener) Log(r slog.Record) { //nolint:gocritic // hugeParam
	if s.OPLogCb != nil {
		s.OPLogCb(r)
	}
}

func (s *SelectiveListener) OnAnvilErr(err error) {
	if s.OnAnvilErrCb != nil {
		s.OnAnvilErrCb(err)
	}
}
