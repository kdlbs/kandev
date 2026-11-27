package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.4
func TestTaskGitHubIssueSQLiteEffects(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "effects-ws", Name: "Effects"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "effect-task", WorkspaceID: "effects-ws", Title: "Current", WorkflowID: "effect-wf", WorkflowStepID: "source", AssigneeAgentProfileID: "runner", Priority: "high"}))
	dispatches := 0
	repo.SetStepEntryDispatcher(fieldEntryProbe(func(context.Context, string, string, string, string, int64) { dispatches++ }))
	t.Cleanup(func() { repo.SetStepEntryDispatcher(nil) })
	target := "target"
	pending, ok := stepentry.BuildPendingAllocation(target, []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterClearDecisions}})
	require.True(t, ok)
	holder := &stepentry.AllocationResult{}
	effectCtx := stepentry.WithResultHolder(stepentry.WithPendingAllocation(ctx, pending), holder)
	_, err := repo.UpdateTaskFieldsWithParentAdmission(effectCtx, "effect-task", models.TaskFieldUpdate{WorkflowStepID: &target}, hierarchy.ValidateParent)
	require.NoError(t, err)
	require.Positive(t, holder.EntryID)
	require.Positive(t, holder.TransitionID)
	require.Equal(t, 1, dispatches)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "effect-session", TaskID: "effect-task", AgentProfileID: "runner", State: models.TaskSessionStateCreated}))
	before, err := repo.GetTask(ctx, "effect-task")
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, "effect-session")
	require.NoError(t, err)
	counts := make(map[string]int)
	for _, table := range []string{"task_step_transitions", "workflow_step_entries", "workflow_step_participants"} {
		var count int
		require.NoError(t, repo.DB().QueryRow(`SELECT count(*) FROM `+table+` WHERE task_id='effect-task'`).Scan(&count))
		require.Positive(t, count)
		counts[table] = count
	}
	for _, link := range []*models.TaskGitHubIssueLink{{URL: "url", Number: 42, Owner: "acme", Repo: "api"}, nil} {
		secondHolder := &stepentry.AllocationResult{}
		_, err = repo.UpdateTaskGitHubIssue(stepentry.WithResultHolder(stepentry.WithPendingAllocation(ctx, pending), secondHolder), "effect-task", link)
		require.NoError(t, err)
		require.Zero(t, secondHolder.EntryID)
		require.Zero(t, secondHolder.TransitionID)
		require.Equal(t, 1, dispatches)
		for _, table := range []string{"task_step_transitions", "workflow_step_entries", "workflow_step_participants"} {
			var count int
			require.NoError(t, repo.DB().QueryRow(`SELECT count(*) FROM `+table+` WHERE task_id='effect-task'`).Scan(&count))
			require.Equal(t, counts[table], count, table)
		}
		var runner string
		require.NoError(t, repo.DB().QueryRow(`SELECT agent_profile_id FROM workflow_step_participants WHERE task_id='effect-task' AND role='runner'`).Scan(&runner))
		require.Equal(t, "runner", runner)
		stored, err := repo.GetTask(ctx, "effect-task")
		require.NoError(t, err)
		before.Metadata = stored.Metadata
		before.UpdatedAt = stored.UpdatedAt
		require.Equal(t, before, stored)
		currentSession, err := repo.GetTaskSession(ctx, "effect-session")
		require.NoError(t, err)
		require.Equal(t, session, currentSession)
	}
}
