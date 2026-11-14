package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedSessionlessWIPStep seeds a WIP-1 step holding one FAILED task whose only
// session was cancelled an hour ago, plus one task queued for the step.
func seedSessionlessWIPStep(t *testing.T, repo *sqliterepo.Repository, releaseEnabled bool) (*mockStepGetter, *wfmodels.WorkflowStep) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "release-ws", Name: "Release", WIPSessionlessReleaseEnabled: releaseEnabled, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "release-wf", WorkspaceID: "release-ws", Name: "Release", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	step := &wfmodels.WorkflowStep{ID: "release-ready", WorkflowID: "release-wf", Name: "Execution ready", WIPLimit: 1}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := repo.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, wip_limit, created_at, updated_at)
		VALUES (?, ?, ?, 0, 1, ?, ?)`, step.ID, step.WorkflowID, step.Name, now, now)
	steps := newMockStepGetter()
	steps.steps[step.ID] = step

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "release-stuck", WorkspaceID: "release-ws", WorkflowID: step.WorkflowID, WorkflowStepID: step.ID,
		Title: "Stuck", State: v1.TaskStateFailed, WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("create stuck task: %v", err)
	}
	hourAgo := now.Add(-time.Hour)
	exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, hourAgo, "release-stuck")
	exec(`INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"release-stuck-session", "release-stuck", string(models.TaskSessionStateCancelled), hourAgo, hourAgo)

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "release-next", WorkspaceID: "release-ws", WorkflowID: step.WorkflowID, WorkflowStepID: step.ID,
		Title: "Next", State: v1.TaskStateTODO, QueuedForStepID: step.ID, QueuedAt: &now,
	}); err != nil {
		t.Fatalf("create queued task: %v", err)
	}
	return steps, step
}

func TestReleaseSessionlessWIPSlots_PullsNextQueuedTask(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "off keeps the queue"
		if enabled {
			name = "on pulls the next queued task"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, _ := seedSessionlessWIPStep(t, repo, enabled)
			promotions := 0
			store := newWorkflowStore(
				repo, steps, nil, func(context.Context, *models.Task, ...string) {}, testLogger(), &operationLedger{},
				func(context.Context, *models.Task) { promotions++ },
			)
			service := &Service{workflowStore: store}

			service.releaseSessionlessWIPSlotsOnce(ctx)

			next, err := repo.GetTask(ctx, "release-next")
			if err != nil {
				t.Fatal(err)
			}
			stuck, err := repo.GetTask(ctx, "release-stuck")
			if err != nil {
				t.Fatal(err)
			}
			if stuck.State != v1.TaskStateFailed || !stuck.WIPAdmitted || stuck.WorkflowStepID != "release-ready" {
				t.Fatalf("the sweep changed the sessionless task: %+v", stuck)
			}
			if next.WIPAdmitted != enabled || promotions != boolToInt(enabled) {
				t.Fatalf("next admitted=%v promotions=%d, want admitted=%v", next.WIPAdmitted, promotions, enabled)
			}

			// A second pass never over-admits the step.
			service.releaseSessionlessWIPSlotsOnce(ctx)
			if promotions != boolToInt(enabled) {
				t.Fatalf("repeat sweep promotions = %d", promotions)
			}
		})
	}
}
