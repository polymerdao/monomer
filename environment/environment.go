package environment

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/sourcegraph/conc"
)

type deferFn struct {
	errMsg string
	fn     func() error
}

// Env controls goroutines and deferred functions. Think of it as the "main thread."
// It is not goroutine-safe.
type Env struct {
	deferFns []*deferFn
	wg       *conc.WaitGroup
}

func New() *Env {
	return &Env{
		deferFns: []*deferFn{},
		wg:       conc.NewWaitGroup(),
	}
}

// Go runs fn in a separate goroutine. Close will block until fn returns.
func (e *Env) Go(fn func()) {
	e.wg.Go(fn)
}

// Defer saves fn, which will be run on Close.
func (e *Env) Defer(fn func()) {
	e.DeferErr("", func() error {
		fn()
		return nil
	})
}

// DeferErr saves fn, which will be run on Close.
func (e *Env) DeferErr(errMsg string, fn func() error) {
	e.deferFns = append(e.deferFns, &deferFn{
		errMsg: errMsg,
		fn:     fn,
	})
}

// Close waits for all functions run with Go to finish. Then, it runs all Defer-ed functions.
// The deferred functions are called in reverse order. The Environment must not be used after Close is called.
func (e *Env) Close() error {
	e.wg.Wait()
	var combinedErr error
	for i := 0; i < len(e.deferFns); i++ {
		msgfn := e.deferFns[len(e.deferFns)-1-i]
		if err := msgfn.fn(); err != nil {
			combinedErr = multierror.Append(combinedErr, fmt.Errorf("%s: %w", msgfn.errMsg, err))
		}
	}
	return combinedErr
}
