package runtime

import agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"

// GitOperationResult is the runtime seam's view of an agentctl Git operation.
// The alias keeps lifecycle implementations compatible without making
// higher-level callers import the low-level agentctl package directly.
type GitOperationResult = agentctlclient.GitOperationResult
