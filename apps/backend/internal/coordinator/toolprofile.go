package coordinator

import (
	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// activityTool is the phase-2 read tool over the coordinator's own activity.
const activityTool = "list_coordinator_activity_kandev"

var readTools = []string{
	"list_tasks_kandev", "get_task_conversation_kandev",
	"list_workflows_kandev", "list_workflow_steps_kandev",
	"list_repositories_kandev", "get_coordinator_item_kandev",
}

// improvementTool is the phase-3 tool that proposes a change to the
// coordinator's own context. It exercises no policy action.
const improvementTool = "propose_improvement_kandev"

var proposeTool = map[Action]string{
	ActionCreateTask: "propose_task_kandev",
	ActionMessage:    "propose_message_kandev",
	ActionMove:       "propose_move_kandev",
	ActionResume:     "propose_resume_kandev",
}

// proposeOrder is the fixed order propose tools follow in ToolNames.
var proposeOrder = []Action{ActionCreateTask, ActionMessage, ActionMove, ActionResume}

// ToolNames returns the MCP tools a coordinator conversation may call. With
// phase2 false it is the phase-1 seven; with phase2 true it is the read tools,
// the activity tool, and each propose tool whose action the policy allows;
// phase3 adds the improvement tool.
func ToolNames(p Policy, phase2, phase3 bool) []string {
	names := append([]string{}, readTools...)
	if !phase2 {
		return append(names, proposeTool[ActionCreateTask])
	}
	names = append(names, activityTool)
	for _, a := range proposeOrder {
		if p.Allows(a) {
			names = append(names, proposeTool[a])
		}
	}
	if phase3 {
		names = append(names, improvementTool)
	}
	return names
}

// ActionForTool maps a propose tool to its action; every other name, read
// tools included, is ActionUnknown.
func ActionForTool(name string) Action {
	for _, a := range proposeOrder {
		if proposeTool[a] == name {
			return a
		}
	}
	return ActionUnknown
}

// actionTools maps each coordinator-surface WebSocket action to the MCP tool
// that exposes it. An action absent from the table has no tool.
var actionTools = map[string]string{
	ws.ActionMCPListTasks:            "list_tasks_kandev",
	ws.ActionMCPGetTaskConversation:  "get_task_conversation_kandev",
	ws.ActionMCPListWorkflows:        "list_workflows_kandev",
	ws.ActionMCPListWorkflowSteps:    "list_workflow_steps_kandev",
	ws.ActionMCPListRepositories:     "list_repositories_kandev",
	mcpcontract.ActionGetItem:        "get_coordinator_item_kandev",
	mcpcontract.ActionListActivity:   activityTool,
	mcpcontract.ActionProposeTask:    "propose_task_kandev",
	mcpcontract.ActionProposeResume:  "propose_resume_kandev",
	mcpcontract.ActionProposeMessage: "propose_message_kandev",
	mcpcontract.ActionProposeMove:    "propose_move_kandev",

	mcpcontract.ActionProposeImprovement: improvementTool,
}

// ToolForAction returns the MCP tool name that exposes a WebSocket action.
func ToolForAction(action string) (string, bool) {
	name, ok := actionTools[action]
	return name, ok
}

// ProposeActionFor returns the policy action a propose tool exercises, and
// false for every tool that proposes nothing.
func ProposeActionFor(toolName string) (Action, bool) {
	a := ActionForTool(toolName)
	return a, a != ActionUnknown
}
