package acp

import (
	"errors"
	"strings"
)

// ErrTurnCancelNotAcknowledged means session/cancel (and local prompt interruption)
// were sent but the in-flight session/prompt RPC did not finish within the join
// window. Callers should reconcile local state without assuming the agent stopped.
var ErrTurnCancelNotAcknowledged = errors.New("turn cancel not acknowledged")

// errPromptAbandonedAfterCancel is returned by sendPrompt when a user cancel was
// requested but the session/prompt RPC did not end in time. The prompt gate is
// released so a follow-up prompt can be dispatched.
var errPromptAbandonedAfterCancel = errors.New("prompt abandoned after cancel")

// IsPromptAbandonedAfterCancel reports whether err is the prompt-gate release
// sentinel produced after a user cancel. This is not an agent failure; the
// lifecycle cancel path has already reconciled the interrupted turn.
func IsPromptAbandonedAfterCancel(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errPromptAbandonedAfterCancel) ||
		strings.Contains(err.Error(), errPromptAbandonedAfterCancel.Error())
}
