package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestBackfillExecutorRunningRuntimeRepairsLegacyRows proves empty runtime
// identities are repaired only from persisted executor evidence. Rows without
// evidence stay empty and remain fail-closed.
func TestBackfillExecutorRunningRuntimeRepairsLegacyRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for _, taskID := range []string{"task-runtime-backfill"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Runtime backfill"}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-docker", Name: "Docker", Type: models.ExecutorTypeLocalDocker,
	}); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	for _, session := range []*models.TaskSession{
		{ID: "session-executor-evidence", TaskID: "task-runtime-backfill", State: models.TaskSessionStateRunning},
		{
			ID: "session-snapshot-evidence", TaskID: "task-runtime-backfill", State: models.TaskSessionStateRunning,
			ExecutorSnapshot: map[string]interface{}{"executor_type": string(models.ExecutorTypeWorktree)},
		},
		{ID: "session-no-evidence", TaskID: "task-runtime-backfill", State: models.TaskSessionStateRunning},
		{ID: "session-existing-runtime", TaskID: "task-runtime-backfill", State: models.TaskSessionStateRunning},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}

	legacyRows := []struct {
		id       string
		executor string
	}{
		{id: "session-executor-evidence", executor: "executor-docker"},
		{id: "session-snapshot-evidence", executor: "missing-executor"},
		{id: "session-no-evidence", executor: "missing-executor"},
		{id: "session-existing-runtime", executor: "missing-executor"},
	}
	for _, legacy := range legacyRows {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO executors_running (id, session_id, task_id, executor_id, runtime, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', ?, ?, ?)
		`), legacy.id, legacy.id, "task-runtime-backfill", legacy.executor,
			models.ExecutorRunningStatusStopped, now, now,
		); err != nil {
			t.Fatalf("insert legacy row %s: %v", legacy.id, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE executors_running SET runtime = ? WHERE id = ?`,
	), string(agentruntime.RuntimeStandalone), "session-existing-runtime"); err != nil {
		t.Fatalf("seed existing runtime: %v", err)
	}

	if err := repo.backfillExecutorRunningRuntime(); err != nil {
		t.Fatalf("backfillExecutorRunningRuntime: %v", err)
	}

	assertRuntime := func(sessionID string, want agentruntime.Runtime) {
		t.Helper()
		var got string
		if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
			`SELECT COALESCE(runtime, '') FROM executors_running WHERE id = ?`,
		), sessionID).Scan(&got); err != nil {
			t.Fatalf("read runtime for %s: %v", sessionID, err)
		}
		if got != string(want) {
			t.Fatalf("runtime for %s = %q, want %q", sessionID, got, want)
		}
	}
	assertRuntime("session-executor-evidence", agentruntime.RuntimeDocker)
	assertRuntime("session-snapshot-evidence", agentruntime.RuntimeStandalone)
	assertRuntime("session-no-evidence", "")
	assertRuntime("session-existing-runtime", agentruntime.RuntimeStandalone)
}
