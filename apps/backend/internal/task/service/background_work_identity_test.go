package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func newBackgroundWorkSession(t *testing.T) (*Service, context.Context, string, string, backgroundRunLister) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Test WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	return svc, ctx, taskID, sessionID, repo
}

type backgroundRunLister interface {
	ListBackgroundRunsByWorkload(ctx context.Context, sessionID, workloadID string) ([]*models.BackgroundRun, error)
}

// TestRecordBackgroundWorkPreservesLaunchIdentity pins AC-007.1/.5: the
// settlement update has no turn context, but it must not erase the launch
// call, source message, parent, or origin turn recorded at launch time.
func TestRecordBackgroundWorkPreservesLaunchIdentity(t *testing.T) {
	svc, ctx, taskID, sessionID, _ := newBackgroundWorkSession(t)
	parentID := "parent-work"
	launch := streams.WorkloadRunObservation{
		SessionID:       sessionID,
		WorkID:          "shell-1",
		RunID:           "shell-1",
		Kind:            streams.WorkloadKindShell,
		Title:           "CI poller",
		State:           streams.RunStateRunning,
		SourceCallID:    "call-launch",
		SourceMessageID: "message-launch",
		ParentWorkID:    parentID,
		OriginTurnID:    "turn-launch",
		Capabilities: streams.WorkloadCapabilities{
			Discovery: "events_only",
			Output:    "none",
		},
	}
	if _, err := svc.RecordBackgroundWorkloadObservationWithResult(ctx, launch, taskID, sessionID); err != nil {
		t.Fatalf("record launch: %v", err)
	}

	settled := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    "shell-1",
		RunID:     "shell-1",
		Kind:      streams.WorkloadKindShell,
		Title:     "gh pr checks 4821 --watch",
		State:     streams.RunStateUnknown,
	}
	committed, err := svc.RecordBackgroundWorkloadObservationWithResult(ctx, settled, taskID, sessionID)
	if err != nil {
		t.Fatalf("record settlement: %v", err)
	}

	stored, err := svc.GetBackgroundWorkload(ctx, sessionID, "shell-1")
	if err != nil || stored == nil {
		t.Fatalf("GetBackgroundWorkload: %v %v", stored, err)
	}
	if stored.SourceCallID != "call-launch" || stored.SourceMessageID != "message-launch" {
		t.Fatalf("launch source identity erased: %+v", stored)
	}
	if stored.ParentWorkID == nil || *stored.ParentWorkID != parentID {
		t.Fatalf("parent identity erased: %+v", stored.ParentWorkID)
	}
	if stored.OriginTurnID != "turn-launch" {
		t.Fatalf("origin turn erased: %q", stored.OriginTurnID)
	}
	if stored.Capabilities == "" {
		t.Fatal("capability declaration was erased by a capability-less update")
	}
	if committed.SourceCallID != "call-launch" || committed.OriginTurnID != "turn-launch" {
		t.Fatalf("published projection lost launch identity: %+v", committed)
	}
	if committed.Capabilities.Output != "none" {
		t.Fatalf("published projection lost the declaration: %+v", committed.Capabilities)
	}
}

// TestRecordBackgroundWorkKeepsProviderRunIdentity pins that only an explicit
// provider run ID creates a run. A normalized update without one must not
// invent a second run keyed by the workload ID.
func TestRecordBackgroundWorkKeepsProviderRunIdentity(t *testing.T) {
	svc, ctx, taskID, sessionID, repo := newBackgroundWorkSession(t)

	start := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    "item-1",
		RunID:     "process-1",
		Kind:      streams.WorkloadKindShell,
		Title:     "sleep 300",
		State:     streams.RunStateRunning,
	}
	if _, err := svc.RecordBackgroundWorkloadObservationWithResult(ctx, start, taskID, sessionID); err != nil {
		t.Fatalf("record native start: %v", err)
	}

	// A normalized command update carries no provider run identity of its own.
	update := start
	update.RunID = ""
	update.Title = "sleep 300"
	if _, err := svc.RecordBackgroundWorkloadObservationWithResult(ctx, update, taskID, sessionID); err != nil {
		t.Fatalf("record normalized update: %v", err)
	}

	workload, err := svc.GetBackgroundWorkload(ctx, sessionID, "item-1")
	if err != nil || workload == nil {
		t.Fatalf("GetBackgroundWorkload: %v %v", workload, err)
	}
	runs, err := repo.ListBackgroundRunsByWorkload(ctx, sessionID, "item-1")
	if err != nil {
		t.Fatalf("ListBackgroundRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want exactly the native process run", len(runs))
	}
	if runs[0].ID != "process-1" {
		t.Fatalf("run ID = %q, want process-1", runs[0].ID)
	}
}
