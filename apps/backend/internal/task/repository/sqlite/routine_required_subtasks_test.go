package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRoutineRequiredAssignmentsBlockEveryCompletionWrite(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-required", Name: "Office"}))
	_, err := repo.EnsureOfficeWorkflow(ctx, "ws-required")
	require.NoError(t, err)
	wf, err := repo.EnsureRoutineWorkflow(ctx, "ws-required")
	require.NoError(t, err)
	var start, done string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT id FROM workflow_steps WHERE workflow_id=? AND is_start_step=1`, wf).Scan(&start))
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT id FROM workflow_steps WHERE workflow_id=? AND complete_task_on_enter=1`, wf).Scan(&done))
	parent := &models.Task{ID: "required-parent", WorkspaceID: "ws-required", WorkflowID: wf, WorkflowStepID: start, Title: "Required assignments", State: v1.TaskStateInProgress, Metadata: map[string]interface{}{models.MetaKeyRequiredSubtaskCount: 3}}
	require.NoError(t, repo.CreateTask(ctx, parent))
	snapshot, err := repo.GetTaskCompletionGate(ctx, parent.ID)
	require.NoError(t, err)
	require.True(t, snapshot.Blocked)
	require.Contains(t, snapshot.Blockers, models.TaskCompletionBlocker{CriterionID: "required-subtask-assignments", Reason: "required_assignments_incomplete"})
	require.ErrorContains(t, repo.AddTaskToWorkflow(ctx, parent.ID, wf, done, 0), "required subtask assignments")
	current, err := repo.GetTask(ctx, parent.ID)
	require.NoError(t, err)
	require.Equal(t, start, current.WorkflowStepID, "a workflow move must not enter the completion step")
	batch, err := repo.GetTaskCompletionGateSummaries(ctx, []string{parent.ID})
	require.NoError(t, err)
	require.True(t, batch.ByTaskID[parent.ID].Blocked)
	require.Equal(t, 0, batch.ByTaskID[parent.ID].CriteriaCount, "read-only assignment blockers do not create editable criteria")
	for _, mode := range []string{"state", "state-cas", "workflow-state-cas", "step"} {
		t.Run(mode, func(t *testing.T) {
			switch mode {
			case "state":
				err = repo.UpdateTaskState(ctx, parent.ID, v1.TaskStateCompleted)
			case "state-cas":
				_, _, err = repo.UpdateTaskStateIfCurrentIn(ctx, parent.ID, v1.TaskStateCompleted, []v1.TaskState{v1.TaskStateInProgress})
			case "workflow-state-cas":
				_, err = repo.UpdateTaskStateIfWorkflowStep(ctx, parent.ID, start, v1.TaskStateCompleted)
			case "step":
				copy := *parent
				copy.WorkflowStepID = done
				err = repo.UpdateTask(ctx, &copy)
			}
			require.ErrorContains(t, err, "required subtask assignments")
			current, readErr := repo.GetTask(ctx, parent.ID)
			require.NoError(t, readErr)
			require.Equal(t, v1.TaskStateInProgress, current.State)
			require.Equal(t, start, current.WorkflowStepID)
		})
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("required-child-%d", i)
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: parent.WorkspaceID, WorkflowID: wf, WorkflowStepID: start, ParentID: parent.ID, Title: id, State: v1.TaskStateInProgress}))
	}
	require.ErrorContains(t, repo.UpdateTaskState(ctx, parent.ID, v1.TaskStateCompleted), "required subtask assignments", "unassigned children are not assignments")
	_, err = repo.db.ExecContext(ctx, `UPDATE workflow_steps SET agent_profile_id='default-agent' WHERE id=?`, start)
	require.NoError(t, err)
	require.ErrorContains(t, repo.UpdateTaskState(ctx, parent.ID, v1.TaskStateCompleted), "required subtask assignments", "a workflow default is not a durable task-specific runner assignment")
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("required-child-%d", i)
		seatRow(t, repo, "runner-"+id, start, id, "runner", false)
	}
	require.NoError(t, repo.UpdateTaskState(ctx, parent.ID, v1.TaskStateCompleted))
	current, err = repo.GetTask(ctx, parent.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, current.State)
	snapshot, err = repo.GetTaskCompletionGate(ctx, parent.ID)
	require.NoError(t, err)
	require.False(t, snapshot.Blocked)
	// Already completed tasks remain terminal and are never reopened by validation.
	_, err = repo.db.ExecContext(ctx, `DELETE FROM workflow_step_participants WHERE task_id LIKE 'required-child-%'`)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskState(ctx, parent.ID, v1.TaskStateCompleted))
	batch, err = repo.GetTaskCompletionGateSummaries(ctx, []string{parent.ID})
	require.NoError(t, err)
	require.Nil(t, batch.ByTaskID[parent.ID], "completed tasks retain the legacy empty-gate read shape")
}

func TestUpdateTaskStateIfWorkflowStepRequiresCurrentStep(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-state-cas", Name: "Office"}))
	wf, err := repo.EnsureOfficeWorkflow(ctx, "ws-state-cas")
	require.NoError(t, err)
	var start string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT id FROM workflow_steps WHERE workflow_id=? AND is_start_step=1`, wf).Scan(&start))
	task := &models.Task{ID: "state-cas-task", WorkspaceID: "ws-state-cas", WorkflowID: wf, WorkflowStepID: start, Title: "State CAS"}
	require.NoError(t, repo.CreateTask(ctx, task))

	updated, err := repo.UpdateTaskStateIfWorkflowStep(ctx, task.ID, "stale-step", v1.TaskStateCompleted)
	require.NoError(t, err)
	require.False(t, updated)

	updated, err = repo.UpdateTaskStateIfWorkflowStep(ctx, task.ID, start, v1.TaskStateCompleted)
	require.NoError(t, err)
	require.True(t, updated)
	current, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, current.State)
}

func TestUpdateTaskStateIfWorkflowStepAndStateChecksBothInputs(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-state-step-cas", Name: "Office"}))
	wf, err := repo.EnsureOfficeWorkflow(ctx, "ws-state-step-cas")
	require.NoError(t, err)
	var stepID string
	require.NoError(t, repo.db.QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id=? AND is_start_step=1`, wf,
	).Scan(&stepID))

	type stateStepWriter interface {
		UpdateTaskStateIfWorkflowStepAndState(
			context.Context, string, string, v1.TaskState, v1.TaskState,
		) (bool, error)
	}

	for _, tc := range []struct {
		name          string
		taskID        string
		currentState  v1.TaskState
		expectedState v1.TaskState
		expectedStep  string
		wantApplied   bool
		wantState     v1.TaskState
	}{
		{
			name:         "matching state and step apply",
			taskID:       "state-step-cas-match",
			currentState: v1.TaskStateInProgress, expectedState: v1.TaskStateInProgress,
			expectedStep: "step-current", wantApplied: true, wantState: v1.TaskStateCompleted,
		},
		{
			name:         "blocked state is preserved",
			taskID:       "state-step-cas-blocked",
			currentState: v1.TaskStateBlocked, expectedState: v1.TaskStateInProgress,
			expectedStep: "step-current", wantState: v1.TaskStateBlocked,
		},
		{
			name:         "step mismatch is rejected",
			taskID:       "state-step-cas-stale-step",
			currentState: v1.TaskStateInProgress, expectedState: v1.TaskStateInProgress,
			expectedStep: "step-stale", wantState: v1.TaskStateInProgress,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &models.Task{
				ID: tc.taskID, WorkspaceID: "ws-state-step-cas", WorkflowID: wf,
				WorkflowStepID: stepID, Title: "State and step CAS", State: tc.currentState,
			}
			require.NoError(t, repo.CreateTask(ctx, task))

			writer, ok := any(repo).(stateStepWriter)
			require.True(t, ok, "task repository must expose a state-and-step compare-and-set")
			expectedStepID := tc.expectedStep
			if expectedStepID == "step-current" {
				expectedStepID = stepID
			}
			applied, err := writer.UpdateTaskStateIfWorkflowStepAndState(
				ctx, task.ID, expectedStepID, tc.expectedState, v1.TaskStateCompleted,
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantApplied, applied)
			current, err := repo.GetTask(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantState, current.State)
		})
	}
}

func TestLegacyRequiredSubtaskMetadataDoesNotChangeCompletion(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-legacy-requirement", Name: "Office"}))
	wf, err := repo.EnsureRoutineWorkflow(ctx, "ws-legacy-requirement")
	require.NoError(t, err)
	var start string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT id FROM workflow_steps WHERE workflow_id=? AND is_start_step=1`, wf).Scan(&start))
	for _, tc := range []struct {
		name     string
		metadata string
	}{
		{name: "legacy number", metadata: `{"required_subtask_count":3}`},
		{name: "legacy string", metadata: `{"required_subtask_count":"three"}`},
		{name: "empty metadata", metadata: ""},
		{name: "whitespace metadata", metadata: " \n\t "},
		{name: "malformed legacy metadata", metadata: "not-json"},
		{name: "json null metadata", metadata: "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "legacy-" + strings.ReplaceAll(tc.name, " ", "-")
			parent := &models.Task{ID: id, WorkspaceID: "ws-legacy-requirement", WorkflowID: wf, WorkflowStepID: start, Title: "Existing task", State: v1.TaskStateInProgress}
			require.NoError(t, repo.CreateTask(ctx, parent))
			_, err := repo.db.ExecContext(ctx, `UPDATE tasks SET metadata=? WHERE id=?`, tc.metadata, id)
			require.NoError(t, err)

			detail, err := repo.GetTaskCompletionGate(ctx, id)
			require.NoError(t, err)
			require.False(t, detail.Blocked)
			batch, err := repo.GetTaskCompletionGateSummaries(ctx, []string{id})
			require.NoError(t, err)
			require.Nil(t, batch.ByTaskID[id])
			require.NoError(t, repo.UpdateTaskState(ctx, id, v1.TaskStateCompleted))
		})
	}
}

func TestRequiredSubtaskCountFailsClosedForMalformedTrustedMetadata(t *testing.T) {
	for _, raw := range []string{
		`{"kandev.office_required_subtask_count":"three"}`,
		`{"kandev.office_required_subtask_count":`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := requiredSubtaskCount(raw)
			require.ErrorContains(t, err, "read required subtask assignments")
		})
	}
}
