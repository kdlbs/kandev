package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// failingSessionStopper is a TaskExecutionStopper whose StopSession always
// fails with a genuine (non-"already complete") error, forcing
// executeTaskResourceCleanupJob's failedStops set to be non-empty.
type failingSessionStopper struct{}

func (failingSessionStopper) StopTask(context.Context, string, string, bool) error { return nil }
func (failingSessionStopper) RegisterExecutionStopOwner(string, string, bool)      {}
func (failingSessionStopper) StopSession(context.Context, string, string, bool) error {
	return errors.New("stop failed")
}
func (failingSessionStopper) StopExecution(context.Context, string, string, bool) error { return nil }

// sessionSelectiveFailingStopper fails StopSession only for the named
// session, so a test can exercise a mixed stop outcome on one attempt.
type sessionSelectiveFailingStopper struct{ failSessionID string }

func (sessionSelectiveFailingStopper) StopTask(context.Context, string, string, bool) error {
	return nil
}
func (sessionSelectiveFailingStopper) RegisterExecutionStopOwner(string, string, bool) {}
func (s sessionSelectiveFailingStopper) StopSession(_ context.Context, sessionID, _ string, _ bool) error {
	if sessionID == s.failSessionID {
		return errors.New("stop failed")
	}
	return nil
}
func (sessionSelectiveFailingStopper) StopExecution(context.Context, string, string, bool) error {
	return nil
}

// AC-TASKS-ORPHAN-REAP-006.2: a failed runtime stop gates the reap
// phase off entirely for this attempt — no host snapshot read, no roots, no
// candidate records or skips.
func TestExecuteTaskResourceCleanupJobSkipsReapPhaseOnFailedStop(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const taskID = "task-failed-stop"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-orphan-reap", Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-failed-stop", SessionID: "sess-failed-stop", TaskID: taskID, ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	svc.executionStopper = failingSessionStopper{}
	svc.orphanReapHostSnapshotter = poisonOrphanReapHostSnapshotter{t: t}

	job := &models.TaskResourceCleanupJob{
		ID: "job-failed-stop", TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerDelete,
	}
	snapshot := &taskResourceCleanupSnapshot{}
	err := svc.executeTaskResourceCleanupJob(ctx, job, snapshot)

	if err == nil {
		t.Fatal("expected an error reporting the failed runtime stop")
	}
	if len(snapshot.OrphanReapRoots) != 0 {
		t.Fatalf("expected no reap roots recorded when a stop failed, got %+v", snapshot.OrphanReapRoots)
	}
	if len(snapshot.OrphanReapRecords) != 0 {
		t.Fatalf("expected no candidate records when a stop failed, got %+v", snapshot.OrphanReapRecords)
	}
	if len(snapshot.OrphanReapSkips) != 0 {
		t.Fatalf("expected no skips when a stop failed, got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-001.1: recording a removed path as a reap root is not
// gated on a clean overall stop — only the reap phase's signal-sending is
// (AC-TASKS-ORPHAN-REAP-006.2). A multi-session task with a mixed stop
// outcome removes every non-preserved session's directory on this attempt
// regardless of the failure elsewhere; a directory actually removed here must
// still be recorded, since it will no longer exist to re-derive candidacy
// from on a later attempt once the failing session's stop succeeds.
func TestExecuteTaskResourceCleanupJobRecordsReapRootForSucceededSessionDespiteMixedStopOutcome(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const taskID = "task-mixed-stop"
	const failSessionID = "sess-mixed-fail"
	const okSessionID = "sess-mixed-ok"

	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-orphan-reap", Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, sessionID := range []string{failSessionID, okSessionID} {
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "exec-" + sessionID, SessionID: sessionID, TaskID: taskID, ExecutorID: "executor-1",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning(%s): %v", sessionID, err)
		}
		if err := os.MkdirAll(filepath.Join(quickChatDir, sessionID), 0o755); err != nil {
			t.Fatalf("mkdir session dir: %v", err)
		}
	}
	svc.executionStopper = sessionSelectiveFailingStopper{failSessionID: failSessionID}
	svc.orphanReapHostSnapshotter = poisonOrphanReapHostSnapshotter{t: t}

	okDir := filepath.Join(quickChatDir, okSessionID)
	// Resolve while the directory still exists — resolveOrphanReapPathBestEffort
	// falls back to an unresolved absolute path once it is gone, so the
	// resolved form must be captured before the cleanup below removes it.
	wantRoot := resolveOrphanReapPathBestEffort(okDir)

	job := &models.TaskResourceCleanupJob{
		ID: "job-mixed-stop", TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerDelete,
	}
	snapshot := &taskResourceCleanupSnapshot{}
	err := svc.executeTaskResourceCleanupJob(ctx, job, snapshot)

	if err == nil {
		t.Fatal("expected an error reporting the failed runtime stop")
	}
	if _, statErr := os.Stat(okDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected the succeeded session's directory to be removed, got %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(quickChatDir, failSessionID)); statErr != nil {
		t.Fatalf("expected the failed session's directory to remain: %v", statErr)
	}
	found := false
	for _, root := range snapshot.OrphanReapRoots {
		if root == wantRoot {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the removed directory %q to be recorded as a reap root even though the "+
			"reap phase's signal-sending step is gated off this attempt; got roots=%+v", wantRoot, snapshot.OrphanReapRoots)
	}
	// The signal-sending phase itself is still gated off this attempt
	// (AC-TASKS-ORPHAN-REAP-006.2): no candidate records or skips yet, and
	// the poison snapshotter above would have failed the test had the host
	// snapshot been read.
	if len(snapshot.OrphanReapRecords) != 0 {
		t.Fatalf("expected no candidate records this attempt, got %+v", snapshot.OrphanReapRecords)
	}
}
