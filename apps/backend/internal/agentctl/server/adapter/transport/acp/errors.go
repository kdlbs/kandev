package acp

import "errors"

// ErrTurnCancelNotAcknowledged means session/cancel (and local prompt interruption)
// were sent but the in-flight session/prompt RPC did not finish within the join
// window. Callers should reconcile local state without assuming the agent stopped.
var ErrTurnCancelNotAcknowledged = errors.New("turn cancel not acknowledged")

// errPromptAbandonedAfterCancel is returned by sendPrompt when a user cancel was
// requested but the session/prompt RPC did not end in time. The prompt gate is
// released so a follow-up prompt can be dispatched.
var errPromptAbandonedAfterCancel = errors.New("prompt abandoned after cancel")
