package service

import (
	"context"
	"errors"
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
