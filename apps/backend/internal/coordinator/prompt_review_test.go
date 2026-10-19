package coordinator

import (
	"strings"
	"testing"
)

func TestStandingInstructionsDeferAutomaticExecutionToPolicyAndGuard(t *testing.T) {
	got := StandingInstructions("workspace", "ws", "coordinator", "")
	for _, want := range []string{"manager-configured permissions", "A human must decide each proposal that requires approval", "Automatic approvals never start agents", "Only the tool guard"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions omit policy boundary %q: %s", want, got)
		}
	}
	if strings.Contains(got, "proposals are never applied automatically") {
		t.Fatal("instructions contradict a manager's automatic permission")
	}
}
