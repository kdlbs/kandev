package client

import "errors"

const agentPromptAction = "agent.prompt"

// promptUnsentError is used only before any WebSocket write is attempted.
type promptUnsentError struct{ error }

func (e *promptUnsentError) Unwrap() error { return e.error }

// PromptDefinitelyUnsent excludes response loss and write ambiguity.
func PromptDefinitelyUnsent(err error) bool {
	var unsent *promptUnsentError
	return errors.As(err, &unsent)
}
