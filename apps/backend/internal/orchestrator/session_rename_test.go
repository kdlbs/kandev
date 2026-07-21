package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRenameSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := svc.RenameSession(ctx, "session1", "  reviewer  "); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.Name != "reviewer" {
		t.Errorf("expected trimmed name %q, got %q", "reviewer", session.Name)
	}

	// Over-long names are truncated, not rejected.
	long := strings.Repeat("x", maxSessionNameLength+50)
	if err := svc.RenameSession(ctx, "session1", long); err != nil {
		t.Fatalf("RenameSession long: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if len(session.Name) != maxSessionNameLength {
		t.Errorf("expected name capped at %d chars, got %d", maxSessionNameLength, len(session.Name))
	}

	// Unknown sessions surface an error to the WS caller.
	if err := svc.RenameSession(ctx, "missing-session", "x"); err == nil {
		t.Fatal("expected error for missing session")
	}
}
