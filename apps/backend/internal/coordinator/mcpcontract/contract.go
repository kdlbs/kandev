// Package mcpcontract holds the names the coordinator's MCP tool surface and
// the coordinator service share. It imports nothing, so the MCP server that
// agentctl embeds can use it without pulling in the coordinator service.
package mcpcontract

// ActionProposeTask is the MCP action the propose_task_kandev tool dispatches.
const ActionProposeTask = "coordinator.propose_task"

// Proposal spec field names, matching their JSON keys.
const (
	FieldWorkflowID = "workflow_id"
	FieldStepID     = "step_id"
)
