package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestHandledLaunchFailureLeavesTypedErrorOwnerWithoutLegacyGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)

	messages := &mockMessageCreator{}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.messageCreator = messages
	err := errors.New("environment preparation failed: fatal: couldn't find remote ref feature/deleted")
	if got := svc.handleSessionLaunchFailure(ctx, "task1", "session1", err); !errors.Is(got, err) {
		t.Fatalf("handleSessionLaunchFailure error = %v, want %v", got, err)
	}

	if len(messages.sessionMessages) != 0 {
		t.Fatalf("legacy launch guidance messages = %d, want 0", len(messages.sessionMessages))
	}
	if _, suppressed := svc.suppressToast.Load("session1"); suppressed {
		t.Fatal("typed launch failure must not suppress the pointer toast")
	}
	session, getErr := repo.GetTaskSession(ctx, "session1")
	if getErr != nil {
		t.Fatalf("get failed session: %v", getErr)
	}
	if session.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %s, want FAILED", session.State)
	}
	if _, claimed := session.Metadata["missing_pr_branch_recovery_claimed"]; claimed {
		t.Fatal("legacy missing-branch claim must not be written")
	}
}
