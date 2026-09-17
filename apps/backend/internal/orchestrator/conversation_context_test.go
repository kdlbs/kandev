package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestNativeConversationColdResetClearsResumeMetadata(t *testing.T) {
	repo := setupTestRepo(t)
	seedOfficeTaskAndSessions(t, repo)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "s-assignee", "acp_session_id", "old-context"); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "t-office", AssigneeAgentProfileID: "agent-assignee", Metadata: map[string]interface{}{"native_conversation": true}}
	if err := svc.resetNativeConversation(ctx, task); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "s-assignee")
	if err != nil {
		t.Fatal(err)
	}
	if session.Metadata["acp_session_id"] != "" {
		t.Fatal("cold launch would restore the previous provider conversation")
	}
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.resetNativeConversation(ctx, task); err == nil {
		t.Fatal("must not reset an active coordinator turn")
	}
}
