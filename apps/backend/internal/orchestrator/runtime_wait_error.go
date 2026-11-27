package orchestrator

import (
	"context"
	"errors"
)

// sessionRuntimeWaitExpiry identifies expiry of a readiness wait's own context.
// A provider deadline returned while that context is still live is not marked.
type sessionRuntimeWaitExpiry struct{ cause error }

func (e *sessionRuntimeWaitExpiry) Error() string { return e.cause.Error() }
func (e *sessionRuntimeWaitExpiry) Unwrap() error { return e.cause }

func markSessionRuntimeWaitExpiry(waitCtx context.Context, err error) error {
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		return err
	}
	return &sessionRuntimeWaitExpiry{cause: err}
}
