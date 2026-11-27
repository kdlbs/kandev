package coordinator

import "testing"

// Every tool a conversation can be bound to must be reachable through some
// action, or the execution guard refuses each call as not_in_profile.
func TestToolForAction_CoversEveryBoundTool(t *testing.T) {
	p := PhaseOnePolicy()
	for _, a := range AllActions {
		p.Actions[a] = SettingRequiresApproval
	}
	reachable := map[string]bool{}
	for _, tool := range actionTools {
		reachable[tool] = true
	}
	for _, phase2 := range []bool{false, true} {
		for _, tool := range ToolNames(p, phase2, false) {
			if !reachable[tool] {
				t.Errorf("phase2=%v: tool %q is bound but no action maps to it", phase2, tool)
			}
		}
	}
}

func TestToolForAction_ListActivity(t *testing.T) {
	tool, ok := ToolForAction(ActionListActivity)
	if !ok || tool != "list_coordinator_activity_kandev" {
		t.Fatalf("ToolForAction(ActionListActivity) = %q, %v", tool, ok)
	}
}
