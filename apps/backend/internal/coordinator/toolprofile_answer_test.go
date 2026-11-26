package coordinator

import (
	"slices"
	"testing"
)

func TestToolNamesAnswerRequiresPhase3AndAllowedClass(t *testing.T) {
	allowed := PhaseOnePolicy()
	allowed.Actions[ActionAnswer] = SettingRequiresApproval
	denied := PhaseOnePolicy()
	denied.Actions[ActionAnswer] = SettingDenied

	cases := []struct {
		name   string
		policy Policy
		p3     bool
		want   bool
	}{
		{"allowed with phase 3", allowed, true, true},
		{"allowed without phase 3", allowed, false, false},
		{"denied with phase 3", denied, true, false},
		{"absent entry with phase 3", PhaseOnePolicy(), true, false},
	}
	for _, tc := range cases {
		if got := slices.Contains(ToolNames(tc.policy, true, tc.p3), answerTool); got != tc.want {
			t.Errorf("%s: listed = %v, want %v", tc.name, got, tc.want)
		}
	}
	if slices.Contains(ToolNames(allowed, false, true), answerTool) {
		t.Error("phase 1 must not list the answer tool")
	}
}

func TestAnswerToolMapsBackToItsAction(t *testing.T) {
	if got := ActionForTool(answerTool); got != ActionAnswer {
		t.Fatalf("ActionForTool = %q", got)
	}
	if name, ok := ToolForAction(ActionProposeAnswer); !ok || name != answerTool {
		t.Fatalf("ToolForAction = %q, %v", name, ok)
	}
}
