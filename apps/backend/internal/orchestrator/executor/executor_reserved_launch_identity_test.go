package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPrepareSessionUsesReservedLaunchIdentityBeforeAgentEffects(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{ID: "repo-1", SourceType: sourceTypeLocal})
	exec := newPreflightTestExecutor(t, repo)
	identity := models.ReservedLaunchIdentity{TaskID: "task-1", SessionID: "reserved-session", TurnID: "reserved-turn", SubmissionID: "reserved-submission"}
	ctx := models.WithReservedLaunchIdentity(context.Background(), identity)
	_, err := exec.PrepareSession(ctx, &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, "agent-profile", "", "", "")
	if err != nil {
		t.Fatalf("prepare reserved session: %v", err)
	}
	if len(repo.createTaskSessionCalls) != 1 || repo.createTaskSessionCalls[0].ID != identity.SessionID {
		t.Fatalf("session persistence did not use reserved identity: %#v", repo.createTaskSessionCalls)
	}
	identity.TaskID = "other-task"
	_, err = exec.PrepareSession(models.WithReservedLaunchIdentity(context.Background(), identity), &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, "agent-profile", "", "", "")
	if err == nil {
		t.Fatal("foreign reserved task was accepted")
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatal("foreign identity created a session")
	}
}
