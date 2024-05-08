package environment_test

import (
	"errors"
	"testing"

	"github.com/polymerdao/monomer/environment"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestGo(t *testing.T) {
	env := environment.New()
	ch := make(chan struct{})
	env.Go(func() {
		ch <- struct{}{}
	})
	<-ch
	require.NoError(t, env.Close())
}

func TestDefer(t *testing.T) {
	env := environment.New()
	wasRun := false
	env.Defer(func() {
		wasRun = true
	})
	require.NoError(t, env.Close())
	require.True(t, wasRun)
}

func TestDeferErr(t *testing.T) {
	env := environment.New()
	err := errors.New("test")
	env.DeferErr("", func() error {
		return err
	})
	require.ErrorIs(t, env.Close(), err)
}

func TestClose(t *testing.T) {
	// Ensure the goroutines spawned with Go are stopped before deferred funcs are executed.
	env := environment.New()
	goCh := make(chan struct{})
	env.Go(func() {
		goCh <- struct{}{}
	})
	deferCh := make(chan struct{})
	env.Defer(func() {
		deferCh <- struct{}{}
	})

	var wg conc.WaitGroup
	defer wg.Wait()
	wg.Go(func() {
		require.NoError(t, env.Close())
	})
	<-goCh
	<-deferCh
}
