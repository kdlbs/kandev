package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestHandledLaunchFailureLeavesTypedErrorOwnerWithoutLegacyGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	messages := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = messages
	err := errors.New("environment preparation failed: fatal: couldn't find remote ref feature/deleted")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

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
	if _, claimed := session.Metadata["missing_pr_branch_recovery_claimed"]; claimed {
		t.Fatal("legacy missing-branch claim must not be written")
	}
}
