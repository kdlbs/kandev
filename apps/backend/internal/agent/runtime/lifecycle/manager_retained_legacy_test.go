package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestEnsureDurableExecutionSelectionAllowsHistoricalRow proves a terminal
// inventory row is retained history, not a live worker that can contradict a
// new selection.
func TestEnsureDurableExecutionSelectionAllowsHistoricalRow(t *testing.T) {
	for _, status := range []string{
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete,
		models.ExecutorRunningStatusFailed,
	} {
		t.Run(status, func(t *testing.T) {
			mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
			mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
				ID: "session-historical", SessionID: "session-historical", TaskID: "task-1",
				Runtime: agentruntime.RuntimeStandalone, Status: status,
			}})

			if err := mgr.ensureDurableExecutionSelectionCompatible(
				context.Background(), "session-historical", "env-1", string(models.ExecutorTypeLocalDocker), true,
			); err != nil {
				t.Fatalf("historical row error = %v, want nil", err)
			}
		})
	}
}

// TestEnsureDurableExecutionSelectionAllowsPredecessorEnvironment proves an
// authorized environment transition is not blocked by its predecessor's
// inventory row. The same-environment mismatch stays refused.
func TestEnsureDurableExecutionSelectionAllowsPredecessorEnvironment(t *testing.T) {
	prior := &models.ExecutorRunning{
		ID: "session-transition", SessionID: "session-transition", TaskID: "task-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusReady,
		Metadata: map[string]interface{}{MetadataKeyTaskEnvironmentID: "env-predecessor"},
	}
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: prior})

	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-transition", "env-selected", string(models.ExecutorTypeLocalDocker), true,
	); err != nil {
		t.Fatalf("predecessor environment error = %v, want nil", err)
	}
	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-transition", "env-predecessor", string(models.ExecutorTypeLocalDocker), true,
	); !errors.Is(err, ErrExecutorRuntimeMismatch) {
		t.Fatalf("same-environment error = %v, want mismatch", err)
	}
}

// TestRetainedEnvironmentMismatchReportsEnvironment proves an environment
// mismatch is not reported as a runtime-versus-runtime contradiction.
func TestRetainedEnvironmentMismatchReportsEnvironment(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	execution := &AgentExecution{
		ID: "execution-legacy", SessionID: "session-legacy", TaskID: "task-1",
		TaskEnvironmentID: "env-old", ExecutorType: string(models.ExecutorTypeLocalDocker),
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	err := mgr.validateRetainedExecutionRuntime(context.Background(), execution, executor.NameDocker, "env-new")
	if !errors.Is(err, ErrExecutorRuntimeMismatch) {
		t.Fatalf("error = %v, want runtime mismatch", err)
	}
	if !strings.Contains(err.Error(), "task environment") || strings.Contains(err.Error(), "standalone\" does not match observed") {
		t.Fatalf("error = %v, want environment-specific detail", err)
	}
}

type writeOnlyRunningWriter struct{}

func (writeOnlyRunningWriter) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}

func (writeOnlyRunningWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}

func (writeOnlyRunningWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

// TestEnsureDurableSelectionFailsClosedWithoutReader proves retained evidence
// is refused when a configured writer cannot read durable inventory.
func TestEnsureDurableSelectionFailsClosedWithoutReader(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(writeOnlyRunningWriter{})

	err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-no-reader", "env-1", string(models.ExecutorTypeLocalDocker), true,
	)
	if !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("error = %v, want identity unverified", err)
	}
}

type flakyRetainedInventoryWriter struct {
	failures int
	reads    int
	prior    *models.ExecutorRunning
}

func (w *flakyRetainedInventoryWriter) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	w.reads++
	if w.reads <= w.failures {
		return nil, errors.New("transient inventory read")
	}
	if w.prior == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	return w.prior, nil
}

func (w *flakyRetainedInventoryWriter) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}

func (w *flakyRetainedInventoryWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}

func (w *flakyRetainedInventoryWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

// TestRecoveredAvailabilityRetriesTransientInventoryRead proves a transient
// identity-read failure does not permanently suppress the recovered Ready
// event, while a genuine mismatch is not retried.
func TestRecoveredAvailabilityRetriesTransientInventoryRead(t *testing.T) {
	execution := &AgentExecution{
		ID: "execution-recovery", SessionID: "session-recovery", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocal),
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	flaky := &flakyRetainedInventoryWriter{
		failures: 2,
		prior: &models.ExecutorRunning{
			ID: "session-recovery", SessionID: "session-recovery", TaskID: "task-1",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusReady,
		},
	}
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(flaky)
	if !mgr.recoveredAvailabilityCompatible(context.Background(), execution) {
		t.Fatal("recovered availability was suppressed after transient read failures")
	}
	if flaky.reads != 3 {
		t.Fatalf("inventory reads = %d, want 3", flaky.reads)
	}

	mismatch := &flakyRetainedInventoryWriter{
		prior: &models.ExecutorRunning{
			ID: "session-recovery", SessionID: "session-recovery", TaskID: "task-1",
			Runtime: agentruntime.RuntimeDocker, Status: models.ExecutorRunningStatusReady,
		},
	}
	mgr, _ = newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(mismatch)
	if mgr.recoveredAvailabilityCompatible(context.Background(), execution) {
		t.Fatal("mismatched runtime was reported compatible")
	}
	if mismatch.reads != 1 {
		t.Fatalf("mismatch inventory reads = %d, want 1", mismatch.reads)
	}
}
