package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBackgroundWorkOutputBounds(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Test WS"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID,
		Kind:      streams.WorkloadKindShell,
		Title:     "large output test",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	if err := svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID); err != nil {
		t.Fatalf("RecordBackgroundWorkloadObservation: %v", err)
	}

	// Append large output chunks exceeding 200KB limit
	chunk1 := strings.Repeat("a", 150*1024)
	chunk2 := strings.Repeat("b", 100*1024)

	err := svc.AppendBackgroundWorkloadOutput(ctx, streams.WorkloadOutputChunk{
		WorkID: workID,
		Chunk:  chunk1,
		Offset: int64(len(chunk1)),
	}, sessionID)
	if err != nil {
		t.Fatalf("AppendBackgroundWorkloadOutput chunk 1: %v", err)
	}

	err = svc.AppendBackgroundWorkloadOutput(ctx, streams.WorkloadOutputChunk{
		WorkID: workID,
		Chunk:  chunk2,
		Offset: int64(len(chunk1) + len(chunk2)),
	}, sessionID)
	if err != nil {
		t.Fatalf("AppendBackgroundWorkloadOutput chunk 2: %v", err)
	}

	got, err := svc.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		t.Fatalf("GetBackgroundWorkload: %v", err)
	}
	if got == nil {
		t.Fatalf("expected workload, got nil")
	}

	const maxOutputLength = 200 * 1024
	if len(got.Output) != maxOutputLength {
		t.Fatalf("output length = %d, want %d", len(got.Output), maxOutputLength)
	}
	if !got.OutputTruncated {
		t.Fatalf("expected OutputTruncated = true")
	}
	if !strings.HasSuffix(got.Output, chunk2) {
		t.Fatalf("expected output to end with chunk2")
	}
}

func TestBackgroundWorkServiceObservations(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Test WS"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID:    sessionID,
		WorkID:       workID,
		Kind:         streams.WorkloadKindSubagent,
		Title:        "code reviewer",
		State:        streams.RunStateRunning,
		OriginTurnID: "turn-1",
		SourceCallID: "call-1",
		StartedAt:    &now,
		Revision:     1,
	}

	if err := svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID); err != nil {
		t.Fatalf("RecordBackgroundWorkloadObservation: %v", err)
	}

	list, err := svc.ListBackgroundWorkloads(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListBackgroundWorkloads: %v", err)
	}
	if len(list) != 1 || list[0].ID != workID {
		t.Fatalf("unexpected workloads list: %#v", list)
	}

	// Monotonic state transition
	finished := now.Add(5 * time.Second)
	exitCode := 0
	obs.State = streams.RunStateCompleted
	obs.FinishedAt = &finished
	obs.ExitCode = &exitCode
	obs.Revision = 2

	if err := svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID); err != nil {
		t.Fatalf("RecordBackgroundWorkloadObservation terminal: %v", err)
	}

	got, err := svc.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		t.Fatalf("GetBackgroundWorkload: %v", err)
	}
	if got.State != "completed" || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("unexpected workload state: %#v", got)
	}
}

func TestBackgroundWorkReadAuthorization(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "A Workspace", OwnerID: "user-a"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID,
		Kind:      streams.WorkloadKindShell,
		Title:     "private work",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	if err := svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID); err != nil {
		t.Fatalf("RecordBackgroundWorkloadObservation: %v", err)
	}

	// Internal or owner access succeeds
	ownerCtx := ctxAs("user-a")
	list, err := svc.ListBackgroundWorkloads(ownerCtx, sessionID)
	if err != nil || len(list) != 1 {
		t.Fatalf("owner ListBackgroundWorkloads: got %v, err %v", list, err)
	}
	got, err := svc.GetBackgroundWorkload(ownerCtx, sessionID, workID)
	if err != nil || got == nil {
		t.Fatalf("owner GetBackgroundWorkload: got %v, err %v", got, err)
	}

	// Unauthorized user access is denied
	otherCtx := ctxAs("user-b")
	_, err = svc.ListBackgroundWorkloads(otherCtx, sessionID)
	if err == nil {
		t.Fatalf("expected unauthorized user to fail ListBackgroundWorkloads")
	}
	_, err = svc.GetBackgroundWorkload(otherCtx, sessionID, workID)
	if err == nil {
		t.Fatalf("expected unauthorized user to fail GetBackgroundWorkload")
	}
}
