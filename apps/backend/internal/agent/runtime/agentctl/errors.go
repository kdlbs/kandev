package client

import (
	acptransport "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
)

// ErrTurnCancelNotAcknowledged means the agent did not end the in-flight prompt
// after a cancel request within the join window.
var ErrTurnCancelNotAcknowledged = acptransport.ErrTurnCancelNotAcknowledged
