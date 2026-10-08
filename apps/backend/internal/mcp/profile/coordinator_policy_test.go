package profile

import (
	"slices"
	"testing"
)

func TestBoundCoordinatorToolNamesFallsBackToPhaseOneSeven(t *testing.T) {
	names := BoundCoordinatorToolNames(Context{})
	if len(names) != 7 || !slices.Contains(names, "propose_task_kandev") {
		t.Fatalf("phase-1 names = %v", names)
	}
	bound := Context{CoordinatorToolPolicy: &CoordinatorToolPolicy{ToolNames: []string{"list_tasks_kandev"}}}
	if got := BoundCoordinatorToolNames(bound); len(got) != 1 {
		t.Fatalf("bound names = %v", got)
	}
}
