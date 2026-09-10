package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStopSessionSynchronouslyRegistersTeardownOwner(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-sync-owner"] = &models.TaskSession{
		ID: "session-sync-owner", TaskID: "task-sync-owner", State: models.TaskSessionStateRunning,
	}
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-sync-owner", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error { return nil },
	}
	exec := newTestExecutor(t, manager, repo)
	var claimed bool
	exec.SetOnExecutionStopOwnerRegistration(func(sessionID, executionID string, force bool) {
		if sessionID != "session-sync-owner" || executionID != "execution-sync-owner" || force {
			t.Fatalf("stop claim = (%q, %q, %v)", sessionID, executionID, force)
		}
		claimed = true
	})

	if err := exec.StopSessionSynchronously(
		context.Background(), "session-sync-owner", "archive", false,
	); err != nil {
		t.Fatalf("StopSessionSynchronously: %v", err)
	}
	if !claimed {
		t.Fatal("synchronous stop did not register teardown ownership")
	}
}
