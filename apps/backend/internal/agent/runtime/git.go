package runtime

import agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"

// GitOperationResult is the runtime seam's view of an agentctl Git operation.
// The alias keeps lifecycle implementations compatible without making
// higher-level callers import the low-level agentctl package directly.
type GitOperationResult = agentctlclient.GitOperationResult

// GitLogResult, CumulativeDiffResult, and GitStatusResult keep Git observation
// contracts behind the public runtime seam.
type GitLogResult = agentctlclient.GitLogResult
type CumulativeDiffResult = agentctlclient.CumulativeDiffResult
type GitStatusResult = agentctlclient.GitStatusResult
