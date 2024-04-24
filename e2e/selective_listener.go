package e2e

import (
	"io"

	"github.com/ethereum/go-ethereum/log"
)

type SelectiveListener struct {
	OPLogWithPrefixCb  func(prefix string, r *log.Record)
	AfterOPStartupCb   func()
	BeforeOPShutdownCb func()

	OnCmdStartCb   func(programName string, stdout, stderr io.Reader)
	OnCmdStoppedCb func(programName string, err error)
}

func (s *SelectiveListener) LogWithPrefix(prefix string, r *log.Record) {
	if s.OPLogWithPrefixCb != nil {
		s.OPLogWithPrefixCb(prefix, r)
	}
}

func (s *SelectiveListener) AfterStartup() {
	if s.AfterOPStartupCb != nil {
		s.AfterOPStartupCb()
	}
}

func (s *SelectiveListener) BeforeShutdown() {
	if s.BeforeOPShutdownCb != nil {
		s.BeforeOPShutdownCb()
	}
}

func (s *SelectiveListener) OnCmdStart(programName string, stdout, stderr io.Reader) {
	if s.OnCmdStartCb != nil {
		s.OnCmdStartCb(programName, stdout, stderr)
	}
}

func (s *SelectiveListener) OnCmdStopped(programName string, err error) {
	if s.OnCmdStartCb != nil {
		s.OnCmdStoppedCb(programName, err)
	}
}
