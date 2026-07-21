package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestProcessActivityKind(t *testing.T) {
	tests := []struct {
		kind string
		want activity.Kind
	}{
		{kind: "setup", want: activity.KindSetupScript},
		{kind: "cleanup", want: activity.KindCleanupScript},
		{kind: "test", want: activity.KindTestCommand},
		{kind: "custom", want: activity.KindShellCommand},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			if got := processActivityKind(test.kind); got != test.want {
				t.Fatalf("processActivityKind(%q) = %q, want %q", test.kind, got, test.want)
			}
		})
	}
}

func TestTerminalProcessStatusReleasesTrackedActivity(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	manager := &Manager{}
	manager.SetActivityCoordinator(coordinator)

	lease, err := manager.acquireActivity(context.Background(), activity.KindShellCommand)
	if err != nil {
		t.Fatal(err)
	}
	manager.trackActivity(processActivityKey("process-1"), lease)
	if len(coordinator.BusyKinds()) != 1 {
		t.Fatal("expected process activity to hold the host gate")
	}

	manager.releaseTerminalProcessActivity(&agentctltypes.ProcessStatusUpdate{
		ProcessID: "process-1",
		Status:    agentctltypes.ProcessStatusExited,
	})
	if busy := coordinator.BusyKinds(); len(busy) != 0 {
		t.Fatalf("terminal process left busy resources: %v", busy)
	}

	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer maintenance.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.acquireActivity(ctx, activity.KindExecutionStarting); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireActivity error = %v, want context.Canceled", err)
	}
}

func TestMarkCompletedReleasesTrackedExecutionActivity(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	execution := &AgentExecution{ID: "execution-complete", Status: v1.AgentStatusRunning}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("Add execution: %v", err)
	}
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionRunning)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	manager.trackActivity(executionActivityKey(execution.ID), lease)

	if err := manager.MarkCompleted(execution.ID, 0, ""); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if err != nil {
		t.Fatalf("maintenance after MarkCompleted: %v", err)
	}
}

func TestMarkBootReadyReleasesTrackedInitialExecutionActivity(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	execution := &AgentExecution{ID: "execution-no-prompt", Status: v1.AgentStatusRunning}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("Add execution: %v", err)
	}
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionPreparing)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	manager.trackActivity(executionActivityKey(execution.ID), lease)

	if err := manager.MarkBootReady(execution.ID); err != nil {
		t.Fatalf("MarkBootReady: %v", err)
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if err != nil {
		t.Fatalf("maintenance after MarkBootReady: %v", err)
	}
}

func TestStartAgentProcessFailureReleasesTrackedExecutionActivity(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	execution := &AgentExecution{ID: "execution-start-failure", Status: v1.AgentStatusStarting}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("Add execution: %v", err)
	}
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionPreparing)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	manager.trackActivity(executionActivityKey(execution.ID), lease)

	if err := manager.StartAgentProcess(context.Background(), execution.ID); err == nil {
		t.Fatal("StartAgentProcess returned nil, want missing agentctl error")
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if err != nil {
		t.Fatalf("maintenance after startup failure: %v", err)
	}
}

func TestInitialPromptWaitFailureRetainsExecutionActivity(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionRunning)
	if err != nil {
		t.Fatal(err)
	}
	manager.trackActivity(executionActivityKey("execution-prompt-wait"), lease)

	manager.sessionManager.SetInitialPromptFailureHandler(func(executionID string) {
		if executionID != "execution-prompt-wait" {
			t.Errorf("initial prompt failure execution ID = %q", executionID)
		}
	})
	manager.sessionManager.initialPromptFailure("execution-prompt-wait")
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy while execution may still run", err)
	}
	manager.releaseActivity(executionActivityKey("execution-prompt-wait"))
}

func TestReleaseCancelsPendingExecutionActivityAcquire(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("TryAcquireMaintenance: %v", err)
	}

	acquired := make(chan error, 1)
	go func() {
		_, acquireErr := manager.ensureExecutionActivity(
			context.Background(), "completed-while-acquiring", activity.KindExecutionStarting,
		)
		acquired <- acquireErr
	}()
	<-maintenance.Context().Done()
	manager.releaseActivity(executionActivityKey("completed-while-acquiring"))
	maintenance.Release()
	if err := <-acquired; !errors.Is(err, errExecutionActivityInvalidated) {
		t.Fatalf("ensureExecutionActivity error = %v, want invalidation", err)
	}

	next, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("late execution lease remained active: %v", err)
	}
	next.Release()
}

func TestCancelledConcurrentExecutionActivityAcquireDoesNotInvalidateLeader(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("TryAcquireMaintenance: %v", err)
	}

	leaderDone := make(chan error, 1)
	go func() {
		claim, acquireErr := manager.ensureExecutionActivity(
			context.Background(), "shared-start", activity.KindExecutionStarting,
		)
		if acquireErr == nil {
			claim.Commit()
		}
		leaderDone <- acquireErr
	}()
	select {
	case <-maintenance.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("leader did not reach activity admission")
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	claim, err := manager.ensureExecutionActivity(
		cancelledCtx, "shared-start", activity.KindExecutionStarting,
	)
	claim.Release()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("concurrent acquire error = %v, want context cancellation", err)
	}
	maintenance.Release()
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader acquire: %v", err)
	}
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0); !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy after leader acquired", err)
	}
	manager.releaseActivity(executionActivityKey("shared-start"))
}

func TestStaleFailedExecutionActivityDoesNotReleaseSuccessfulSuccessor(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)

	stale, err := manager.ensureExecutionActivity(
		context.Background(), "shared-start", activity.KindExecutionStarting,
	)
	if err != nil {
		t.Fatalf("stale acquire: %v", err)
	}
	successor, err := manager.ensureExecutionActivity(
		context.Background(), "shared-start", activity.KindExecutionPreparing,
	)
	if err != nil {
		t.Fatalf("successor acquire: %v", err)
	}

	successor.Commit()
	stale.Release()
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0); !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy after successor succeeded", err)
	}
	manager.releaseActivity(executionActivityKey("shared-start"))
}
