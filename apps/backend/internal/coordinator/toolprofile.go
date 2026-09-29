package coordinator

// activityTool is the phase-2 read tool over the coordinator's own activity.
const activityTool = "list_coordinator_activity_kandev"

var readTools = []string{
	"list_tasks_kandev", "get_task_conversation_kandev",
	"list_workflows_kandev", "list_workflow_steps_kandev",
	"list_repositories_kandev", "get_coordinator_item_kandev",
}

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
// the activity tool, and each propose tool whose action the policy allows.
func ToolNames(p Policy, phase2 bool) []string {
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
