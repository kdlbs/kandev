package backendapp

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBootWorkflowSnapshotTaskCoverage(t *testing.T) {
	h := newBootStateTestHarness(t)
	workspaces, err := h.taskSvc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := h.taskSvc.ListWorkflows(t.Context(), workspaces[0].ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	workflow := workflows[0]
	steps, err := h.workflowSvc.ListStepsByWorkflow(t.Context(), workflow.ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	b := bootStateBuilder{p: routeParams{taskSvc: h.taskSvc, services: &Services{Workflow: h.workflowSvc}}}
	snapshot, ok := b.workflowSnapshotState(t.Context(), workflow)
	require.True(t, ok)
	coverage := snapshot["taskCoverage"].(*models.TaskCoverage)
	require.True(t, coverage.Complete)
	require.Zero(t, coverage.Total)
	require.Equal(t, "sqlite_nocase_v1", coverage.OrderingProfile)
	for _, item := range []struct{ id, step string }{{"shown", steps[0].ID}, {"without-step", ""}} {
		require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{
			ID: item.id, WorkspaceID: workflow.WorkspaceID, WorkflowID: workflow.ID, WorkflowStepID: item.step, Title: item.id,
		}))
	}
	snapshot, ok = b.workflowSnapshotState(t.Context(), workflow)
	require.True(t, ok)
	coverage = snapshot["taskCoverage"].(*models.TaskCoverage)
	require.False(t, coverage.Complete, "boot omitted the task without a step")
	require.Equal(t, 2, coverage.Total)
	require.Len(t, snapshot["tasks"], 1)
	_, err = h.db.Exec("UPDATE tasks SET archived_at = ? WHERE id = ?", time.Now(), "without-step")
	require.NoError(t, err)
	snapshot, ok = b.workflowSnapshotState(t.Context(), workflow)
	require.True(t, ok)
	coverage = snapshot["taskCoverage"].(*models.TaskCoverage)
	require.True(t, coverage.Complete)
	require.Equal(t, 1, coverage.Total, "archived tasks do not establish active coverage")
}
