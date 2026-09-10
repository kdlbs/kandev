package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type failAutoStartOnCreateClaimRepository struct {
	sessionExecutorStore
}

func (r *failAutoStartOnCreateClaimRepository) RemoveTaskMetadataKey(ctx context.Context, taskID, key string) (bool, error) {
	if key == models.MetaKeyAutoStartOnCreate {
		return false, errors.New("injected auto-start claim failure")
	}
	return r.sessionExecutorStore.RemoveTaskMetadataKey(ctx, taskID, key)
}

func TestAutoStartOnCreateClaimLeavesDurableInFlightMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-inflight", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-inflight", WorkspaceID: "ws-inflight", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-inflight", WorkspaceID: "ws-inflight", WorkflowID: "wf-inflight",
		WorkflowStepID: "step-inflight", State: v1.TaskStateCreated,
		Metadata:  map[string]interface{}{models.MetaKeyAutoStartOnCreate: true},
		CreatedAt: now, UpdatedAt: now,
	}))

	svc := &Service{repo: repo, logger: testLogger()}
	task, err := repo.GetTask(ctx, "task-inflight")
	requireNoError(t, err)
	if got := svc.claimAutoStartOnCreateForLaunch(ctx, task, false); got != autoStartOnCreateClaimOwned {
		t.Fatalf("claim result = %d, want owned", got)
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	requireNoError(t, err)
	if models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("auto-start-on-create intent remained after claim")
	}
	if !models.HasAutoStartOnCreateInFlight(reloaded.Metadata) {
		t.Fatal("in-flight marker was not persisted before launch")
	}
	if got := svc.claimAutoStartOnCreateForLaunch(ctx, reloaded, false); got != autoStartOnCreateClaimLost {
		t.Fatalf("second claim result = %d, want lost", got)
	}

	svc.completeAutoStartOnCreate(ctx, task.ID, "test")
	reloaded, err = repo.GetTask(ctx, task.ID)
	requireNoError(t, err)
	if models.HasAutoStartOnCreateIntent(reloaded.Metadata) || models.HasAutoStartOnCreateInFlight(reloaded.Metadata) {
		t.Fatalf("auto-start markers remained after completion: %#v", reloaded.Metadata)
	}
}

func TestAutoStartOnCreateClaimReclaimsStaleInFlightMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-stale", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-stale", WorkspaceID: "ws-stale", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-stale", WorkspaceID: "ws-stale", WorkflowID: "wf-stale",
		WorkflowStepID: "step-stale", State: v1.TaskStateCreated,
		Metadata:  map[string]interface{}{models.MetaKeyAutoStartOnCreateInFlight: true},
		CreatedAt: now, UpdatedAt: now,
	}))

	svc := &Service{repo: repo, logger: testLogger()}
	task, err := repo.GetTask(ctx, "task-stale")
	requireNoError(t, err)
	if got := svc.claimAutoStartOnCreateForLaunch(ctx, task, false); got != autoStartOnCreateClaimOwned {
		t.Fatalf("stale claim result = %d, want owned", got)
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	requireNoError(t, err)
	if models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("stale recovery left the rebuilt intent in place")
	}
	if !models.HasAutoStartOnCreateInFlight(reloaded.Metadata) {
		t.Fatal("stale recovery did not create a fresh in-flight marker")
	}
}

func TestAutoStartOnCreateClaimFailureSkipsLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-claim-error", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-claim-error", WorkspaceID: "ws-claim-error", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-claim-error", WorkspaceID: "ws-claim-error", WorkflowID: "wf-claim-error",
		WorkflowStepID: "step-claim-error", State: v1.TaskStateCreated,
		Metadata:  map[string]interface{}{models.MetaKeyAutoStartOnCreate: true},
		CreatedAt: now, UpdatedAt: now,
	}))

	step := &wfmodels.WorkflowStep{
		ID: "step-claim-error", WorkflowID: "wf-claim-error", Name: "Start", Position: 0,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps[step.ID] = step
	failingRepo := &failAutoStartOnCreateClaimRepository{sessionExecutorStore: repo}
	svc := &Service{repo: failingRepo, workflowStepGetter: stepGetter, logger: testLogger()}

	task, err := repo.GetTask(ctx, "task-claim-error")
	requireNoError(t, err)
	// The claim must fail closed before any goroutine or executor call. The
	// executor field is deliberately unset, so a launch would panic this test.
	svc.autoStartTaskForLoadedStep(ctx, task, step, "test.claim_error", false, 0, false)

	reloaded, err := repo.GetTask(ctx, task.ID)
	requireNoError(t, err)
	if !models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("claim failure removed the original intent")
	}
	if !models.HasAutoStartOnCreateInFlight(reloaded.Metadata) {
		t.Fatal("claim failure did not leave a recoverable in-flight marker")
	}
}
