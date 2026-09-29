package runtime

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// AgentCtlClient is the runtime-bound agentctl client used by approved
// low-level adapters such as authenticated reverse proxies.
type AgentCtlClient = agentctl.Client

// RuntimeOwner and AvailabilitySnapshot expose sanitized local runtime
// ownership and availability through the runtime package boundary.
type RuntimeOwner = agentctl.RuntimeOwner
type AvailabilitySnapshot = agentctl.AvailabilitySnapshot

type AgentctlEventPayload = lifecycle.AgentctlEventPayload

var (
	ErrRecoveryConflict     = agentctl.ErrRecoveryConflict
	ErrRecoveryNotRetryable = agentctl.ErrRecoveryNotRetryable
	ErrRecoveryRequestID    = agentctl.ErrRecoveryRequestID
	ErrRuntimeOwnerStopped  = agentctl.ErrRuntimeOwnerStopped
)
