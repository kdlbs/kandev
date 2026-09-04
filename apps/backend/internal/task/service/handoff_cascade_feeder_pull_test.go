package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recordingCascadeTaskService struct {
	*Service
	vacatedStepIDs []string
}

func (s *recordingCascadeTaskService) ReconcileVacatedStep(ctx context.Context, vacatedStepID string) {
	s.vacatedStepIDs = append(s.vacatedStepIDs, vacatedStepID)
	s.Service.ReconcileVacatedStep(ctx, vacatedStepID)
}

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
func TestHandoffTaskTreeLifecycleBackfillsFeederVacancies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *HandoffService, string) (*CascadeOutcome, error)
	}{
		{name: "archive", mutate: func(ctx context.Context, handoff *HandoffService, taskID string) (*CascadeOutcome, error) {
			return handoff.ArchiveTaskTree(ctx, taskID, true)
		}},
		{name: "delete", mutate: func(ctx context.Context, handoff *HandoffService, taskID string) (*CascadeOutcome, error) {
			return handoff.DeleteTaskTree(ctx, taskID, true)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			taskService, _, repo := createTestService(t)
			seedHandoffFeederPullScenario(t, ctx, taskService, repo)

			recorder := &recordingCascadeTaskService{Service: taskService}
			handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
			handoff.SetTaskEventPublisher(recorder)
			handoff.SetVacatedStepReconciler(recorder)

			outcome, err := tt.mutate(ctx, handoff, "destination-root")
			if err != nil {
				t.Fatalf("tree lifecycle mutation: %v", err)
			}
			if len(outcome.ArchivedTaskIDs) != 2 {
				t.Fatalf("mutated task count = %d, want 2", len(outcome.ArchivedTaskIDs))
			}

			assertHandoffFeederBackfill(t, ctx, repo)
			if len(recorder.vacatedStepIDs) != 1 || recorder.vacatedStepIDs[0] != "review-step" {
				t.Fatalf("vacated step reconciliations = %v, want [review-step]", recorder.vacatedStepIDs)
			}
		})
	}
}

func seedHandoffFeederPullScenario(
	t *testing.T,
	ctx context.Context,
	taskService *Service,
	repo *sqliterepo.Repository,
) {
	t.Helper()
	seedWIPWorkflow(t, ctx, repo)
	taskService.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"waiting-step": {ID: "waiting-step", WorkflowID: "wip-workflow", Position: 0},
		"review-step": {
			ID: "review-step", WorkflowID: "wip-workflow", Position: 1,
			WIPLimit: 2, PullFromStepID: "waiting-step",
		},
		"done-step": {ID: "done-step", WorkflowID: "wip-workflow", Position: 2},
	}})

	tasks := []*models.Task{
		{ID: "destination-root", Title: "Review root", WorkflowStepID: "review-step", Position: 0},
		{ID: "destination-child", Title: "Review child", WorkflowStepID: "review-step", ParentID: "destination-root", Position: 1},
		{ID: "feeder-first", Title: "Waiting first", WorkflowStepID: "waiting-step", Position: 0},
		{ID: "feeder-second", Title: "Waiting second", WorkflowStepID: "waiting-step", Position: 1},
		{ID: "feeder-third", Title: "Waiting third", WorkflowStepID: "waiting-step", Position: 2},
	}
	for _, task := range tasks {
		task.WorkspaceID = "wip-workspace"
		task.WorkflowID = "wip-workflow"
		task.State = v1.TaskStateTODO
		task.Priority = "medium"
		task.WIPAdmitted = true
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}
}

func assertHandoffFeederBackfill(t *testing.T, ctx context.Context, repo *sqliterepo.Repository) {
	t.Helper()
	for _, taskID := range []string{"feeder-first", "feeder-second"} {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		if task.WorkflowStepID != "review-step" || !task.WIPAdmitted || task.QueuedForStepID != "" {
			t.Fatalf("task %s placement = step:%q admitted:%v queued_for:%q, want admitted in review-step",
				taskID, task.WorkflowStepID, task.WIPAdmitted, task.QueuedForStepID)
		}
		trigger, actorKind, _, sessionID := lastLedgerAttribution(t, repo, taskID)
		if trigger != "wip_pull" || actorKind != "system" || sessionID != nil {
			t.Fatalf("task %s ledger = trigger:%q actor:%q session:%v, want wip_pull/system/no session",
				taskID, trigger, actorKind, sessionID)
		}
	}

	remaining, err := repo.GetTask(ctx, "feeder-third")
	if err != nil {
		t.Fatalf("GetTask(feeder-third): %v", err)
	}
	if remaining.WorkflowStepID != "waiting-step" || !remaining.WIPAdmitted || remaining.QueuedForStepID != "" {
		t.Fatalf("remaining feeder placement = step:%q admitted:%v queued_for:%q, want admitted in waiting-step",
			remaining.WorkflowStepID, remaining.WIPAdmitted, remaining.QueuedForStepID)
	}
}
