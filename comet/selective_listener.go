package comet

type SelectiveListener struct {
	OnSubscriptionWriteErrCb func(error)
	OnSubscriptionCanceledCb func(error)
}

func (s *SelectiveListener) OnSubscriptionWriteErr(err error) {
	if s.OnSubscriptionWriteErrCb != nil {
		s.OnSubscriptionWriteErrCb(err)
	}
}

func (s *SelectiveListener) OnSubscriptionCanceled(err error) {
	if s.OnSubscriptionCanceledCb != nil {
		s.OnSubscriptionCanceledCb(err)
	}
}
