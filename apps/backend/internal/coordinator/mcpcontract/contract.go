// Package mcpcontract holds the names the coordinator's MCP tool surface and
// the coordinator service share. It imports nothing, so the MCP server that
// agentctl embeds can use it without pulling in the coordinator service.
package mcpcontract

// ActionProposeTask is the MCP action the propose_task_kandev tool dispatches.
const ActionProposeTask = "coordinator.propose_task"

// ActionGetItem is the MCP action the get_coordinator_item_kandev tool
// dispatches (docs/specs/coordinator/system-design/copilot-tools.md#item-read).
const ActionGetItem = "coordinator.get_item"

// ActionApproveProposal and ActionRejectProposal are reserved MCP action
// names (docs/specs/coordinator/system-design/proposals.md#security): approve
// and reject are reachable only through the two REST routes, and no handler
// is ever registered for either name. Reserving them here lets the
// coordinator guard refuse a coordinator or unresolved principal by name,
// before either could reach the dispatcher's own unknown-action answer.
const (
	ActionApproveProposal = "coordinator.approve_proposal"
	ActionRejectProposal  = "coordinator.reject_proposal"
)

// DecisionActions is the set of reserved decision action names the
// coordinator guard checks first, ahead of the propose check and the
// allowlist.
var DecisionActions = map[string]struct{}{
	ActionApproveProposal: {},
	ActionRejectProposal:  {},
}

// Proposal spec field names, matching their JSON keys.
const (
	FieldWorkflowID = "workflow_id"
	FieldStepID     = "step_id"
)
