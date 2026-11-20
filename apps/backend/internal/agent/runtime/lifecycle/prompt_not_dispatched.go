package lifecycle

import "errors"

// ErrPromptNotDispatched marks a prompt failure that happened before any
// agentctl transport I/O, so the agent cannot have received the prompt.
// Callers that recorded an at-most-once delivery marker may safely revoke it
// and retry.
var ErrPromptNotDispatched = errors.New("prompt was not dispatched")

type promptNotDispatchedError struct{ cause error }

// Error returns the cause's text unchanged so classification never alters
// user-visible failure messages.
func (e *promptNotDispatchedError) Error() string { return e.cause.Error() }

func (e *promptNotDispatchedError) Unwrap() error { return e.cause }

func (e *promptNotDispatchedError) Is(target error) bool { return target == ErrPromptNotDispatched }

// MarkPromptNotDispatched classifies err as a failure that happened before
// prompt transport. It returns nil for nil and err itself when already marked.
func MarkPromptNotDispatched(err error) error {
	if err == nil || errors.Is(err, ErrPromptNotDispatched) {
		return err
	}
	return &promptNotDispatchedError{cause: err}
}
