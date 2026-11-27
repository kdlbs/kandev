package coordinator

import (
	"slices"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestToolNamesPhase31_AddsTheTurnsToolOnlyWhenOn(t *testing.T) {
	p := Policy{Version: 1, Actions: allDenied()}
	for _, tc := range []struct{ phase2, phase3, phase31, want bool }{
		{true, true, true, true},
		{true, false, true, true},
		{true, true, false, false},
	} {
		got := slices.Contains(ToolNamesPhase31(p, tc.phase2, tc.phase3, tc.phase31), turnsTool)
		if got != tc.want {
			t.Errorf("%+v: has tool = %v", tc, got)
		}
	}
	if slices.Contains(ToolNames(p, true, true), turnsTool) {
		t.Error("ToolNames must not include the phase 3.1 tool")
	}
}

func TestTurnsToolIsBindableAndMapsToItsAction(t *testing.T) {
	if tool, ok := ToolForAction(ActionListTurns); !ok || tool != turnsTool {
		t.Fatalf("ToolForAction = %q, %v", tool, ok)
	}
	if _, isPropose := ProposeActionFor(turnsTool); isPropose {
		t.Error("the turns tool exercises no policy action")
	}
	policy := mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t",
		ToolNames: ToolNamesPhase31(Policy{Version: 1, Actions: allDenied()}, true, true, true),
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("binding with the turns tool is invalid: %v", err)
	}
}
