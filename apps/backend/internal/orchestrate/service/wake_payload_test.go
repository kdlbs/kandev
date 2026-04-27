package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestBuildWakePayload_WithTaskAndComments(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Insert task.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, identifier, priority, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"t-1", "ws-1", "Build OAuth", "Add login flow", "KAN-42", 3, "IN_PROGRESS")

	// Insert comments.
	svc.ExecSQL(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now', '-2 minutes'))`, "c1", "t-1", "agent", "ceo", "Prioritize this", "user")
	svc.ExecSQL(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now', '-1 minutes'))`, "c2", "t-1", "user", "admin", "LGTM", "user")

	input := &service.WakeupPayloadInput{Payload: `{"task_id":"t-1"}`}
	raw, err := svc.BuildWakePayload(ctx, input)
	if err != nil {
		t.Fatalf("BuildWakePayload: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty payload")
	}

	var payload service.WakePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify task fields.
	if payload.Task == nil {
		t.Fatal("expected task in payload")
	}
	if payload.Task.Title != "Build OAuth" {
		t.Errorf("task title = %q, want %q", payload.Task.Title, "Build OAuth")
	}
	if payload.Task.Identifier != "KAN-42" {
		t.Errorf("task identifier = %q, want %q", payload.Task.Identifier, "KAN-42")
	}

	// Verify comments.
	if len(payload.NewComments) != 2 {
		t.Errorf("comments count = %d, want 2", len(payload.NewComments))
	}
	// Most recent first.
	if len(payload.NewComments) > 0 && payload.NewComments[0].Author != "admin" {
		t.Errorf("first comment author = %q, want %q (newest first)", payload.NewComments[0].Author, "admin")
	}

	// Verify comment window.
	if payload.CommentWindow == nil {
		t.Fatal("expected comment window")
	}
	if payload.CommentWindow.Total != 2 {
		t.Errorf("comment window total = %d, want 2", payload.CommentWindow.Total)
	}
	if payload.CommentWindow.Included != 2 {
		t.Errorf("comment window included = %d, want 2", payload.CommentWindow.Included)
	}
	if payload.CommentWindow.FetchMore {
		t.Error("fetchMore should be false when all comments are included")
	}
}

func TestBuildWakePayload_NoTaskID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	input := &service.WakeupPayloadInput{Payload: `{}`}
	raw, err := svc.BuildWakePayload(ctx, input)
	if err != nil {
		t.Fatalf("BuildWakePayload: %v", err)
	}
	if raw != "" {
		t.Errorf("expected empty payload for no task_id, got %q", raw)
	}
}

func TestBuildWakePayload_TaskNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	input := &service.WakeupPayloadInput{Payload: `{"task_id":"nonexistent"}`}
	raw, err := svc.BuildWakePayload(ctx, input)
	if err != nil {
		t.Fatalf("BuildWakePayload: %v", err)
	}
	if raw != "" {
		t.Errorf("expected empty payload for missing task, got %q", raw)
	}
}

func TestBuildWakePayload_CommentWindowFetchMore(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, "t-2", "ws-1", "Many comments")

	// Insert 7 comments (exceeds limit of 5).
	for i := 0; i < 7; i++ {
		svc.ExecSQL(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
			VALUES (?, ?, ?, ?, ?, ?, datetime('now', ?))`,
			"cm-"+string(rune('a'+i)), "t-2", "agent", "worker", "comment", "user",
			"-"+string(rune('0'+byte(7-i)))+" minutes")
	}

	input := &service.WakeupPayloadInput{Payload: `{"task_id":"t-2"}`}
	raw, err := svc.BuildWakePayload(ctx, input)
	if err != nil {
		t.Fatalf("BuildWakePayload: %v", err)
	}

	var payload service.WakePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.CommentWindow.Total != 7 {
		t.Errorf("total = %d, want 7", payload.CommentWindow.Total)
	}
	if payload.CommentWindow.Included != 5 {
		t.Errorf("included = %d, want 5", payload.CommentWindow.Included)
	}
	if !payload.CommentWindow.FetchMore {
		t.Error("fetchMore should be true when total > included")
	}
}
