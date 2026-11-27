package shared

// AgentTaskRelations carries the relation fields an Office agent may declare
// when it creates a task through the runtime: the tasks the new task depends
// on and an explicit workspace mode. The zero value declares nothing, so an
// omitted field keeps the creation path's existing default.
type AgentTaskRelations struct {
	BlockedBy     []string
	WorkspaceMode string
}
