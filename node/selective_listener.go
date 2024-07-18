package node

type SelectiveListener struct {
	OnEngineHTTPServeErrCb      func(error)
	OnEngineWebsocketServeErrCb func(error)
	OnCometServeErrCb           func(error)
	OnPrometheusServeErrCb      func(error)
}

func (s *SelectiveListener) OnEngineHTTPServeErr(err error) {
	if s.OnEngineHTTPServeErrCb != nil {
		s.OnEngineHTTPServeErrCb(err)
	}
}

func (s *SelectiveListener) OnEngineWebsocketServeErr(err error) {
	if s.OnEngineWebsocketServeErrCb != nil {
		s.OnEngineWebsocketServeErrCb(err)
	}
}

func (s *SelectiveListener) OnCometServeErr(err error) {
	if s.OnCometServeErrCb != nil {
		s.OnCometServeErrCb(err)
	}
}

func (s *SelectiveListener) OnPrometheusServeErr(err error) {
	if s.OnPrometheusServeErrCb != nil {
		s.OnPrometheusServeErrCb(err)
	}
}
