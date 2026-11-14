package models

// AgentWorkspaceCounts contains the two agent totals used by the Office
// workspace overview.
type AgentWorkspaceCounts struct {
	Total   int
	Running int
}
