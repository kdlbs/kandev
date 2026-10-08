package runtime

import agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"

// ProbeResult is the runtime seam's result for background workload probes.
type ProbeResult = agentctlclient.ProbeResult

const (
	ProbeResultLive    = agentctlclient.ProbeResultLive
	ProbeResultSettled = agentctlclient.ProbeResultSettled
	ProbeResultUnknown = agentctlclient.ProbeResultUnknown
)
