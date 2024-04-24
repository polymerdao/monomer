package utils

import (
	"context"
	"errors"
	"fmt"
)

func RunAndWrapOnError(existingErr error, msg string, fn func() error) error {
	if runErr := fn(); runErr != nil {
		runErr = fmt.Errorf("%s: %v", msg, runErr)
		if existingErr == nil {
			return runErr
		}
		return fmt.Errorf("operation failed: %v, previous error: %w", runErr, existingErr)
	}
	return existingErr
}

// Cause returns the error that was supplied to the context's CancelCauseFunc function.
// If no error has been supplied, it returns nil.
func Cause(ctx context.Context) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, ctx.Err()) {
		return nil
	}
	return cause
}

func Ptr[T any](x T) *T {
	return &x
}
