package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestAppendLiveGitStatusMessageReplaysTerminalMaterializedSessionSnapshot
// covers the post-materialization shape written by the executor: the session
// owns the canonical environment path, the agent is terminal so no live source
// is available, and the status notification must still replay its snapshot.
func TestAppendLiveGitStatusMessageReplaysTerminalMaterializedSessionSnapshot(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	const (
		taskID        = "git-status-materialized-task"
		environmentID = "git-status-materialized-environment"
		sessionID     = "git-status-materialized-session"
		canonicalPath = "/tasks/git-status/materialized"
	)
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	if err := harness.taskRepo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            environmentID,
		TaskID:        taskID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: canonicalPath,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                sessionID,
		TaskID:            taskID,
		TaskEnvironmentID: environmentID,
		WorkspacePath:     canonicalPath,
		State:             models.TaskSessionStateCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := harness.taskRepo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-materialized-snapshot",
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files: map[string]interface{}{
			"materialized.go": map[string]interface{}{"status": "modified"},
		},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-31T12:05:00Z",
			"modified":  []string{"materialized.go"},
		},
		CreatedAt: now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	requested, err := harness.taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	msgs := appendLiveGitStatusMessage(ctx, harness.taskRepo, nil, sessionID, requested, nil, newTestLogger())
	if len(msgs) != 1 {
		t.Fatalf("expected one replayed status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["materialized.go"]; !ok {
		t.Fatalf("terminal materialized snapshot was not replayed: files = %#v", files)
	}
}
