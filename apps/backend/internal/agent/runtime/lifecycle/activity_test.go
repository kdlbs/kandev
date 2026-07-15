package lifecycle

import (
	"context"
	"errors"
	"testing"

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

	if handler := manager.sessionManager.initialPromptFailure; handler != nil {
		handler("execution-prompt-wait")
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy while execution may still run", err)
	}
	manager.releaseActivity(executionActivityKey("execution-prompt-wait"))
}

func TestReleaseInvalidatesPendingExecutionActivityAcquire(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("TryAcquireMaintenance: %v", err)
	}

	acquired := make(chan error, 1)
	go func() {
		acquired <- manager.ensureExecutionActivity(
			context.Background(), "completed-while-acquiring", activity.KindExecutionStarting,
		)
	}()
	<-maintenance.Context().Done()
	manager.releaseActivity(executionActivityKey("completed-while-acquiring"))
	maintenance.Release()
	if err := <-acquired; err != nil {
		t.Fatalf("ensureExecutionActivity: %v", err)
	}

	next, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("late execution lease remained active: %v", err)
	}
	next.Release()
}
