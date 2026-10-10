package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestBootAllWorkflowsSeedsOnlyFirstDisplayedBoard(t *testing.T) {
	fixture := newJourneyReadFixture(t, 1)
	state := map[string]any{}
	builder := bootStateBuilder{p: fixture.params}
	builder.addKanbanSnapshotsState(t.Context(), state, []*models.Workflow{
		{ID: "hidden-first", Hidden: true},
		{ID: fixture.workflowID, Name: "Displayed", WorkspaceID: fixture.workspaceID},
		{ID: "offscreen", Name: "Offscreen", WorkspaceID: fixture.workspaceID},
	}, "")
	snapshots := state["kanbanMulti"].(map[string]any)["snapshots"].(map[string]any)
	if len(snapshots) != 1 || snapshots[fixture.workflowID] == nil {
		t.Fatalf("All workflows snapshots = %#v, want only first displayed board", snapshots)
	}
	if state["kanban"].(map[string]any)["workflowId"] != fixture.workflowID {
		t.Fatal("first displayed board did not seed active board context")
	}
}

func TestBootAllWorkflowsSkipsKnownEmptyBoard(t *testing.T) {
	fixture := newJourneyReadFixture(t, 1)
	state := map[string]any{
		"workflows": map[string]any{"taskWorkflowCoverage": &models.TaskWorkflowCoverage{
			WorkspaceID: fixture.workspaceID, WorkflowIDs: []string{fixture.workflowID}, Complete: true,
		}},
	}
	builder := bootStateBuilder{p: fixture.params}
	builder.addKanbanSnapshotsState(t.Context(), state, []*models.Workflow{
		{ID: "empty-first", WorkspaceID: fixture.workspaceID},
		{ID: fixture.workflowID, WorkspaceID: fixture.workspaceID},
	}, "")
	snapshots := state["kanbanMulti"].(map[string]any)["snapshots"].(map[string]any)
	if len(snapshots) != 1 || snapshots[fixture.workflowID] == nil {
		t.Fatalf("snapshots = %#v, want only first nonempty displayed board", snapshots)
	}
}

func TestFirstBootBoardRetainsEmptyFallbackAndSavedColumns(t *testing.T) {
	workflows := []*models.Workflow{{ID: "first"}, {ID: "second"}, {ID: "hidden", Hidden: true}}
	for _, tc := range []struct {
		name  string
		state map[string]any
		want  string
	}{
		{"no coverage", map[string]any{}, "first"},
		{"empty coverage", map[string]any{"workflows": map[string]any{"taskWorkflowCoverage": &models.TaskWorkflowCoverage{Complete: true}}}, "first"},
		{"saved hidden columns", map[string]any{"workflows": map[string]any{"taskWorkflowCoverage": &models.TaskWorkflowCoverage{Complete: true, WorkflowIDs: []string{"second"}}}, "userSettings": map[string]any{"hiddenWorkflowStepIds": map[string][]string{"first": {"hidden-step"}}}}, "first"},
		{"auto-hidden columns", map[string]any{"workflows": map[string]any{"taskWorkflowCoverage": &models.TaskWorkflowCoverage{Complete: true, WorkflowIDs: []string{"second"}}}, "userSettings": map[string]any{"workflowIdsWithAutoHideEmptySteps": []string{"first"}}}, "first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstBootBoardWorkflowID(workflows, tc.state); got != tc.want {
				t.Fatalf("workflow = %q, want %q", got, tc.want)
			}
		})
	}
}
