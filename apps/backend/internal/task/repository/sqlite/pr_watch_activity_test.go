package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestLoadPRWatchTaskActivityUsesAuthoritativeSources(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("initialize queue schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

	seedTask := func(taskID string, createdAt, updatedAt time.Time) {
		t.Helper()
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`), taskID, "workspace-watch-activity", taskID, createdAt, updatedAt); err != nil {
			t.Fatalf("seed task %q: %v", taskID, err)
		}
	}
	seedTask("task-watch-activity", base, base.Add(48*time.Hour))
	seedTask("task-watch-created-only", base.Add(time.Hour), base.Add(72*time.Hour))

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-watch-activity", "task-watch-activity", "RUNNING", base, base); err != nil {
		t.Fatalf("seed running session: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), "turn-watch-lifecycle", "session-watch-activity", "task-watch-activity", base.Add(2*time.Hour), base.Add(3*time.Hour), `{"lifecycle_only":true}`, base.Add(2*time.Hour), base.Add(3*time.Hour)); err != nil {
		t.Fatalf("seed lifecycle turn: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), "turn-watch-agent", "session-watch-activity", "task-watch-activity", base.Add(4*time.Hour), base.Add(5*time.Hour), `{}`, base.Add(4*time.Hour), base.Add(5*time.Hour)); err != nil {
		t.Fatalf("seed agent turn: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "message-watch-user", "session-watch-activity", "task-watch-activity", "turn-watch-agent", "user", "prompt", "message", `{}`, base.Add(6*time.Hour)); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "message-watch-agent", "session-watch-activity", "task-watch-activity", "turn-watch-agent", "agent", "response", "message", `{}`, base.Add(24*time.Hour)); err != nil {
		t.Fatalf("seed agent message: %v", err)
	}
	if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
		ID: "queued-watch-user", SessionID: "session-watch-activity", TaskID: "task-watch-activity",
		Content: "queued prompt", QueuedAt: base.Add(7 * time.Hour), QueuedBy: "user-1",
	}, 10); err != nil {
		t.Fatalf("seed user queue message: %v", err)
	}
	if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
		ID: "queued-watch-agent", SessionID: "session-watch-activity", TaskID: "task-watch-activity",
		Content: "queued lifecycle", QueuedAt: base.Add(36 * time.Hour), QueuedBy: messagequeue.QueuedByAgent,
	}, 10); err != nil {
		t.Fatalf("seed agent queue message: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`), "commit-watch", "session-watch-activity", "sha-watch", base.Add(8*time.Hour), base.Add(8*time.Hour)); err != nil {
		t.Fatalf("seed session commit: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (id, task_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), "environment-watch", "task-watch-activity", base, base); err != nil {
		t.Fatalf("seed task environment: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, head_commit, triggered_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), "snapshot-watch-live", "environment-watch", "session-watch-activity", "status", "main", "live-sha", TriggeredByLiveMonitor, base.Add(30*time.Hour)); err != nil {
		t.Fatalf("seed live snapshot: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, head_commit, triggered_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), "snapshot-watch-push", "environment-watch", "session-watch-activity", "status", "main", "pushed-sha", "agent_completed", base.Add(9*time.Hour)); err != nil {
		t.Fatalf("seed observed push snapshot: %v", err)
	}

	got, err := repo.LoadPRWatchTaskActivity(ctx, []string{
		"task-watch-activity", "task-watch-created-only", "missing-task",
	})
	if err != nil {
		t.Fatalf("load PR watch activity: %v", err)
	}
	activity, ok := got["task-watch-activity"]
	if !ok {
		t.Fatal("activity projection omitted task-watch-activity")
	}
	if !activity.Running {
		t.Fatal("running session was not reported")
	}
	if want := base.Add(9 * time.Hour); !activity.LastActivityAt.Equal(want) {
		t.Fatalf("activity timestamp = %s, want %s", activity.LastActivityAt, want)
	}
	createdOnly, ok := got["task-watch-created-only"]
	if !ok || !createdOnly.LastActivityAt.Equal(base.Add(time.Hour)) {
		t.Fatalf("created-only activity = %+v, want task creation timestamp", createdOnly)
	}
	if _, ok := got["missing-task"]; ok {
		t.Fatal("missing task must not appear in activity projection")
	}

	// A task row update is provider bookkeeping and must not make an idle
	// searching watch look active.
	if createdOnly.LastActivityAt.Equal(base.Add(72 * time.Hour)) {
		t.Fatal("generic task updated_at leaked into activity projection")
	}
}
