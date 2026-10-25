package worktree

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

func TestRecoveryProgressFinishRetriesAfterTransientWriteFailure(t *testing.T) {
	reporter := &failOnceRecoveryProgressReporter{recordingRecoveryProgressReporter: &recordingRecoveryProgressReporter{}, failNextUpdate: true}
	binding, err := reporter.BeginWorkspaceRecovery(context.Background(), RecoveryProgressStart{
		TaskEnvironmentID: "environment-progress-settlement", OwnerTaskID: "task-progress-settlement",
		OwnershipGeneration: 1, SessionID: "session-progress-settlement", OperationID: "operation-progress-settlement",
	})
	if err != nil {
		t.Fatal(err)
	}
	tracker := &recoveryProgressTracker{
		reporter: reporter, binding: binding,
		update: RecoveryProgressUpdate{State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting},
	}
	if err := tracker.updateProgress(context.Background(), RecoveryProgressUpdate{
		State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting,
	}); err == nil {
		t.Fatal("injected progress failure was ignored")
	}
	if err := tracker.finish(context.Background(), recoveryoperation.StateInterrupted, recoveryoperation.PhaseSnapshotting, "storage_recovered", false); err != nil {
		t.Fatalf("terminal settlement after transient write failure: %v", err)
	}
	if !tracker.terminal() {
		t.Fatal("successful terminal write did not end the progress tracker")
	}
	if len(reporter.updates) != 1 {
		t.Fatalf("persisted updates = %+v, want only the successful terminal settlement", reporter.updates)
	}
	terminal := reporter.updates[0]
	if terminal.State != recoveryoperation.StateInterrupted || terminal.ReasonCode != "storage_recovered" {
		t.Fatalf("terminal update = %+v", terminal)
	}
}

type failOnceRecoveryProgressReporter struct {
	*recordingRecoveryProgressReporter
	failNextUpdate bool
}

func (r *failOnceRecoveryProgressReporter) UpdateWorkspaceRecovery(
	ctx context.Context,
	binding RecoveryProgressBinding,
	update RecoveryProgressUpdate,
) (RecoveryProgressBinding, error) {
	if r.failNextUpdate {
		r.failNextUpdate = false
		return RecoveryProgressBinding{}, errors.New("injected progress write failure")
	}
	return r.recordingRecoveryProgressReporter.UpdateWorkspaceRecovery(ctx, binding, update)
}
