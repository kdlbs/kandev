package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type manualMoveFeederPullRecorder struct {
	repo   *sqliterepo.Repository
	called chan struct{}
	err    chan error
	once   sync.Once
}

func (r *manualMoveFeederPullRecorder) ReconcileFeederPulls(ctx context.Context, _, feederStepID string) {
	task, err := r.repo.GetTask(ctx, "manual-feeder-barrier")
	if err != nil {
		r.err <- err
	} else if task.WorkflowStepID != feederStepID {
		r.err <- &unexpectedStepError{got: task.WorkflowStepID, want: feederStepID}
	}
	r.once.Do(func() { close(r.called) })
}

type unexpectedStepError struct {
	got  string
	want string
}

func (e *unexpectedStepError) Error() string { return "feeder pull observed an unexpected task step" }

func TestManualMoveFeederPullWaitsForLifecycleCompletion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "manual-feeder-barrier", "manual-feeder-session", "source-step")
	task, err := repo.GetTask(ctx, "manual-feeder-barrier")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "destination-step"
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.State = v1.TaskStateInProgress
	task.Metadata = map[string]interface{}{
		models.MetaKeyManualMoveLifecyclePending: map[string]interface{}{"from_step_id": "source-step"},
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist moved task: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "manual-feeder-session", "plan_mode", true); err != nil {
		t.Fatalf("seed session metadata: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{
		ID: "source-step", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}}},
	}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type:   wfmodels.OnEnterSetSessionMode,
			Config: map[string]interface{}{"mode": "destination"},
		}}},
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	lifecycleStarted := make(chan struct{})
	releaseLifecycle := make(chan struct{})
	svc.onManualMoveLifecycleStart = func() {
		close(lifecycleStarted)
		<-releaseLifecycle
	}
	recorder := &manualMoveFeederPullRecorder{
		repo: repo, called: make(chan struct{}), err: make(chan error, 1),
	}
	svc.SetFeederPullReconciler(recorder)

	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID: "manual-feeder-barrier", SessionID: "manual-feeder-session",
		FromStepID: "source-step", ToStepID: "destination-step", WIPAdmitted: true,
	})
	select {
	case <-lifecycleStarted:
	case <-time.After(time.Second):
		t.Fatal("manual move lifecycle did not start")
	}
	select {
	case <-recorder.called:
		t.Fatal("feeder pull started before the manual move lifecycle completed")
	default:
	}

	close(releaseLifecycle)
	select {
	case <-recorder.called:
	case <-time.After(time.Second):
		t.Fatal("feeder pull did not start after the manual move lifecycle completed")
	}
	select {
	case err := <-recorder.err:
		t.Fatal(err)
	default:
	}

	stored, err := repo.GetTask(ctx, "manual-feeder-barrier")
	if err != nil {
		t.Fatalf("reload completed move: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; pending {
		t.Fatalf("manual move lifecycle token remained: %#v", stored.Metadata)
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; !completed {
		t.Fatalf("manual move lifecycle completion marker missing: %#v", stored.Metadata)
	}
}
