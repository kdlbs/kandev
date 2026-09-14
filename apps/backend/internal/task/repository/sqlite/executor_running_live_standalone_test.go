package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestListExecutorsRunningLiveStandaloneFiltersRuntimeAndStatus pins the
// startup recovery inventory query (discovery H): only rows on the standalone
// control server (worktree/local) that are not yet terminal are candidates
// for guard-taking and re-tracking correlation at startup step 3, before any
// control-server contact.
func TestListExecutorsRunningLiveStandaloneFiltersRuntimeAndStatus(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedExecutorRunningCleanupTask(t, repo, "task-1")

	seedLiveStandaloneRow(t, repo, "task-1", "session-live-standalone", agentruntime.RuntimeStandalone, models.ExecutorRunningStatusRunning)
	seedLiveStandaloneRow(t, repo, "task-1", "session-stopped-standalone", agentruntime.RuntimeStandalone, models.ExecutorRunningStatusStopped)
	seedLiveStandaloneRow(t, repo, "task-1", "session-completed-standalone", agentruntime.RuntimeStandalone, models.ExecutorRunningStatusComplete)
	seedLiveStandaloneRow(t, repo, "task-1", "session-failed-standalone", agentruntime.RuntimeStandalone, models.ExecutorRunningStatusFailed)
	seedLiveStandaloneRow(t, repo, "task-1", "session-live-docker", agentruntime.RuntimeDocker, models.ExecutorRunningStatusRunning)
	seedLiveStandaloneRow(t, repo, "task-1", "session-live-ssh", agentruntime.RuntimeSSH, models.ExecutorRunningStatusRunning)

	rows, err := repo.ListExecutorsRunningLiveStandalone(ctx)
	if err != nil {
		t.Fatalf("ListExecutorsRunningLiveStandalone: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].SessionID != "session-live-standalone" {
		t.Fatalf("SessionID = %q, want %q", rows[0].SessionID, "session-live-standalone")
	}
	if rows[0].Runtime != agentruntime.RuntimeStandalone {
		t.Fatalf("Runtime = %q, want %q", rows[0].Runtime, agentruntime.RuntimeStandalone)
	}
}

func seedLiveStandaloneRow(
	t *testing.T,
	repo *Repository,
	taskID, sessionID string,
	runtime agentruntime.Runtime,
	status string,
) {
	t.Helper()
	ctx := context.Background()

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateRunning}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		ExecutorID:       "executor-1",
		Runtime:          runtime,
		Status:           status,
		AgentExecutionID: "exec-" + sessionID,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", sessionID, err)
	}
}
