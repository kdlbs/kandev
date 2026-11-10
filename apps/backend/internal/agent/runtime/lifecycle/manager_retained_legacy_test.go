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

// TestInMemoryStandaloneExecutionWithoutInventoryRowStaysCompatible proves a
// live standalone execution remains usable while a local runtime replacement
// has removed its durable row. A Docker boundary still requires the row.
func TestInMemoryStandaloneExecutionWithoutInventoryRowStaysCompatible(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})
	execution := &AgentExecution{
		ID: "execution-standalone", SessionID: "session-standalone", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeWorktree),
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if err := mgr.ValidateExecutionRuntime(
		context.Background(), execution.SessionID, execution.TaskEnvironmentID,
		string(models.ExecutorTypeWorktree),
	); err != nil {
		t.Fatalf("standalone execution without inventory row error = %v, want nil", err)
	}

	docker := &AgentExecution{
		ID: "execution-docker", SessionID: "session-docker", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
		RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container-1",
	}
	if err := mgr.executionStore.Add(docker); err != nil {
		t.Fatalf("add docker execution: %v", err)
	}
	if err := mgr.ValidateExecutionRuntime(
		context.Background(), docker.SessionID, docker.TaskEnvironmentID,
		string(models.ExecutorTypeLocalDocker),
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("docker execution without inventory row error = %v, want identity unverified", err)
	}
}

// TestStandaloneEmptyRuntimeRowStaysCompatible proves an empty durable runtime
// row does not block a standalone execution whose in-memory evidence maps to
// standalone. A Docker boundary still refuses the missing identity.
func TestStandaloneEmptyRuntimeRowStaysCompatible(t *testing.T) {
	execution := &AgentExecution{
		ID: "execution-empty-row", SessionID: "session-empty-row", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeWorktree),
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-empty-row", SessionID: "session-empty-row", TaskID: "task-1",
		Status: models.ExecutorRunningStatusReady,
	}})
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if err := mgr.ValidateExecutionRuntime(
		context.Background(), execution.SessionID, execution.TaskEnvironmentID,
		string(models.ExecutorTypeWorktree),
	); err != nil {
		t.Fatalf("standalone empty runtime row error = %v, want nil", err)
	}

	docker := &AgentExecution{
		ID: "execution-empty-docker", SessionID: "session-empty-docker", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
		RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container-1",
	}
	if err := mgr.executionStore.Add(docker); err != nil {
		t.Fatalf("add docker execution: %v", err)
	}
	if err := mgr.ValidateExecutionRuntime(
		context.Background(), docker.SessionID, docker.TaskEnvironmentID,
		string(models.ExecutorTypeLocalDocker),
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("docker empty runtime row error = %v, want identity unverified", err)
	}
}

// TestEnsureDurableSelectionAllowsMissingStandaloneRow proves a local runtime
// replacement can re-persist a missing inventory row for a standalone
// selection, while a Docker selection still refuses the missing boundary.
func TestEnsureDurableSelectionAllowsMissingStandaloneRow(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})

	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-missing-row", "env-1",
		string(models.ExecutorTypeWorktree), true,
	); err != nil {
		t.Fatalf("missing standalone row error = %v, want nil", err)
	}
	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-missing-row", "env-1",
		string(models.ExecutorTypeLocalDocker), true,
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("missing docker row error = %v, want identity unverified", err)
	}
}

// TestLaunchRequestCarriesRetainedEvidence covers every cold-launch signal
// that must fail closed when durable inventory cannot be read.
func TestLaunchRequestCarriesRetainedEvidence(t *testing.T) {
	tests := []struct {
		name string
		req  *LaunchRequest
		want bool
	}{
		{name: "nil", req: nil, want: false},
		{name: "fresh", req: &LaunchRequest{}, want: false},
		{name: "acp resume", req: &LaunchRequest{ACPSessionID: "acp-1"}, want: true},
		{name: "previous execution", req: &LaunchRequest{PreviousExecutionID: "exec-1"}, want: true},
		{name: "passthrough resume", req: &LaunchRequest{IsPassthrough: true, ResumePassthrough: true}, want: true},
		{name: "passthrough without resume", req: &LaunchRequest{IsPassthrough: true}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := launchRequestCarriesRetainedEvidence(test.req); got != test.want {
				t.Fatalf("retained evidence = %v, want %v", got, test.want)
			}
		})
	}
}

// TestSSHBindingWithoutObservedRuntimeIsRefused proves an in-memory SSH
// binding with no observed runtime cannot borrow its selected type as runtime
// evidence. Docker still requires container identity.
func TestSSHBindingWithoutObservedRuntimeIsRefused(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameSSH})
	ssh := &AgentExecution{
		ID: "execution-ssh", SessionID: "session-ssh", TaskID: "task-1",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeSSH),
	}
	if err := mgr.executionStore.Add(ssh); err != nil {
		t.Fatalf("add ssh execution: %v", err)
	}
	if err := mgr.ValidateExecutionRuntime(
		context.Background(), ssh.SessionID, ssh.TaskEnvironmentID,
		string(models.ExecutorTypeSSH),
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("ssh binding without observed runtime error = %v, want identity unverified", err)
	}
}

// TestSSHEmptyRuntimeRowIsRefused proves an empty runtime row selected as SSH
// stays refused; its selected type is not observed runtime evidence.
func TestSSHEmptyRuntimeRowIsRefused(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameSSH})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-ssh-row", SessionID: "session-ssh-row", TaskID: "task-1",
		Status: models.ExecutorRunningStatusReady,
	}})
	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-ssh-row", "env-1", string(models.ExecutorTypeSSH), true,
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("ssh empty runtime row error = %v, want identity unverified", err)
	}
}

// TestEnsureDurableSelectionAllowsMissingSSHRow proves a stopped remote
// execution with no inventory row can resume through a fresh execution, while
// Docker still refuses the missing container boundary.
func TestEnsureDurableSelectionAllowsMissingSSHRow(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameSSH})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})
	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-missing-ssh", "env-1", string(models.ExecutorTypeSSH), true,
	); err != nil {
		t.Fatalf("missing ssh row error = %v, want nil", err)
	}
	if err := mgr.ensureDurableExecutionSelectionCompatible(
		context.Background(), "session-missing-ssh", "env-1", string(models.ExecutorTypeLocalDocker), true,
	); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("missing docker row error = %v, want identity unverified", err)
	}
}
