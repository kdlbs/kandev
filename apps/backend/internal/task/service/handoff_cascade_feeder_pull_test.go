package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recordingCascadeTaskService struct {
	*Service
	vacatedStepIDs []string
}

type recordingVacatedStepReconciler struct {
	stepIDs []string
}

func (r *recordingVacatedStepReconciler) ReconcileVacatedStep(_ context.Context, stepID string) {
	r.stepIDs = append(r.stepIDs, stepID)
}

type placementChangingCascadeRepo struct {
	*fakeDeleteRepo
}

func (r *placementChangingCascadeRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.fakeDeleteRepo.GetTask(ctx, id)
	if task == nil || err != nil {
		return task, err
	}
	copy := *task
	return &copy, nil
}

func (r *placementChangingCascadeRepo) ArchiveTaskIfActive(
	ctx context.Context,
	id string,
	cascadeID string,
) (bool, error) {
	_, changed, err := r.ArchiveTaskIfActiveWithVacatedStep(ctx, id, cascadeID)
	return changed, err
}

func (r *placementChangingCascadeRepo) ArchiveTaskIfActiveWithVacatedStep(
	_ context.Context,
	id string,
	cascadeID string,
) (string, bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	task := r.base.tasks[id]
	if task == nil || task.ArchivedAt != nil {
		return "", false, nil
	}
	task.WorkflowStepID = "actual-step"
	now := time.Now().UTC()
	task.ArchivedAt = &now
	task.ArchivedByCascadeID = cascadeID
	return task.WorkflowStepID, true, nil
}

func (r *placementChangingCascadeRepo) DeleteTask(ctx context.Context, id string) error {
	_, err := r.DeleteTaskWithVacatedStep(ctx, id)
	return err
}

func (r *placementChangingCascadeRepo) DeleteTaskWithVacatedStep(_ context.Context, id string) (string, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	task := r.base.tasks[id]
	if task == nil {
		return "", errors.New("task not found")
	}
	task.WorkflowStepID = "actual-step"
	delete(r.base.tasks, id)
	return task.WorkflowStepID, nil
}

type partialFailureCascadeRepo struct {
	*fakeDeleteRepo
	failTaskID string
	err        error
}

func (r *partialFailureCascadeRepo) ArchiveTaskIfActive(
	ctx context.Context,
	id string,
	cascadeID string,
) (bool, error) {
	_, changed, err := r.ArchiveTaskIfActiveWithVacatedStep(ctx, id, cascadeID)
	return changed, err
}

func (r *partialFailureCascadeRepo) ArchiveTaskIfActiveWithVacatedStep(
	ctx context.Context,
	id string,
	cascadeID string,
) (string, bool, error) {
	if id == r.failTaskID {
		return "", false, r.err
	}
	task, err := r.GetTask(ctx, id)
	if err != nil || task == nil {
		return "", false, err
	}
	changed, err := r.fakeCascadeRepo.ArchiveTaskIfActive(ctx, id, cascadeID)
	return task.WorkflowStepID, changed, err
}

func (r *partialFailureCascadeRepo) DeleteTask(ctx context.Context, id string) error {
	_, err := r.DeleteTaskWithVacatedStep(ctx, id)
	return err
}

func (r *partialFailureCascadeRepo) DeleteTaskWithVacatedStep(ctx context.Context, id string) (string, error) {
	if id == r.failTaskID {
		return "", r.err
	}
	task, err := r.GetTask(ctx, id)
	if err != nil || task == nil {
		return "", err
	}
	return task.WorkflowStepID, r.fakeDeleteRepo.DeleteTask(ctx, id)
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

func TestHandoffTaskTreeLifecycleUsesMutationPlacementForVacancy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *HandoffService) error
	}{
		{name: "archive", mutate: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.ArchiveTaskTree(ctx, "root", false)
			return err
		}},
		{name: "delete", mutate: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.DeleteTaskTree(ctx, "root", false)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "workspace")
			tasks.tasks["root"].WorkflowStepID = "snapshot-step"
			repo := &placementChangingCascadeRepo{fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}}
			reconciler := &recordingVacatedStepReconciler{}
			handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
			handoff.SetVacatedStepReconciler(reconciler)

			if err := tt.mutate(context.Background(), handoff); err != nil {
				t.Fatalf("tree lifecycle mutation: %v", err)
			}
			if len(reconciler.stepIDs) != 1 || reconciler.stepIDs[0] != "actual-step" {
				t.Fatalf("vacated step reconciliations = %v, want [actual-step]", reconciler.stepIDs)
			}
		})
	}
}

func TestHandoffTaskTreeLifecycleReconcilesCommittedPartialMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *HandoffService) error
	}{
		{name: "archive", mutate: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.ArchiveTaskTree(ctx, "root", true)
			return err
		}},
		{name: "delete", mutate: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.DeleteTaskTree(ctx, "root", true)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "workspace")
			tasks.addTask("child", "root", "workspace")
			tasks.tasks["root"].WorkflowStepID = "review-step"
			tasks.tasks["child"].WorkflowStepID = "review-step"
			mutationErr := errors.New("parent mutation failed")
			repo := &partialFailureCascadeRepo{
				fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
				failTaskID:     "root",
				err:            mutationErr,
			}
			reconciler := &recordingVacatedStepReconciler{}
			handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
			handoff.SetVacatedStepReconciler(reconciler)

			if err := tt.mutate(context.Background(), handoff); !errors.Is(err, mutationErr) {
				t.Fatalf("tree lifecycle error = %v, want %v", err, mutationErr)
			}
			if len(reconciler.stepIDs) != 1 || reconciler.stepIDs[0] != "review-step" {
				t.Fatalf("vacated step reconciliations = %v, want [review-step]", reconciler.stepIDs)
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
