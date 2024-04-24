package utils_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/polymerdao/monomer/utils"
	"github.com/stretchr/testify/require"
)

func TestRunAndWrapOnError(t *testing.T) {
	existingErr := errors.New("existing")
	msg := "msg"
	runErr := errors.New("run")

	// existingErr != nil && runErr != nil
	gotErr := utils.RunAndWrapOnError(existingErr, msg, func() error {
		return runErr
	})
	require.ErrorContains(t, gotErr, fmt.Sprintf("operation failed: %s: %s, previous error: %s", msg, runErr, existingErr))
	require.ErrorIs(t, gotErr, existingErr)
	require.NotErrorIs(t, gotErr, runErr)

	// existingErr == nil && runErr != nil
	gotErr = utils.RunAndWrapOnError(nil, msg, func() error {
		return runErr
	})
	require.ErrorContains(t, gotErr, fmt.Sprintf("%s: %s", msg, runErr))
	require.NotErrorIs(t, gotErr, runErr)

	// existingErr != nil && runErr == nil
	require.Equal(t, utils.RunAndWrapOnError(existingErr, msg, func() error {
		return nil
	}), existingErr)

	// existingErr == nil && runErr == nil
	require.Nil(t, utils.RunAndWrapOnError(nil, msg, func() error {
		return nil
	}))
}

func TestCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	err := errors.New("cause")
	cancel(err)
	require.Equal(t, utils.Cause(ctx), err)

	ctx, cancel = context.WithCancelCause(context.Background())
	cancel(nil)
	require.Nil(t, utils.Cause(ctx))

	ctx, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Nanosecond))
	defer cancelDeadline()
	require.Nil(t, utils.Cause(ctx))
}
