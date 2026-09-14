package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresBootstrapFailureAndExecutorRegistrationShareSessionLock(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedPostgresTaskSession(t, repo, "task-pg-session-lock-upsert", "session-pg-session-lock-upsert")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-pg-session-lock-upsert", SessionID: "session-pg-session-lock-upsert", TaskID: "task-pg-session-lock-upsert",
		AgentExecutionID: "execution-old", Status: models.ExecutorRunningStatusStarting,
	}); err != nil {
		t.Fatalf("seed executor: %v", err)
	}
	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin session lock holder: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, `SELECT id FROM task_sessions WHERE id = $1 FOR UPDATE`, "session-pg-session-lock-upsert"); err != nil {
		t.Fatalf("lock session row: %v", err)
	}

	upsertDone := make(chan error, 1)
	go func() {
		upsertDone <- repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "session-pg-session-lock-upsert", SessionID: "session-pg-session-lock-upsert", TaskID: "task-pg-session-lock-upsert",
			AgentExecutionID: "execution-successor", Status: models.ExecutorRunningStatusStarting,
		})
	}()
	select {
	case err := <-upsertDone:
		t.Fatalf("successor registration completed while session lock was held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("release session lock: %v", err)
	}
	if err := <-upsertDone; err != nil {
		t.Fatalf("successor registration after session lock: %v", err)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-pg-session-lock-upsert")
	if err != nil {
		t.Fatalf("load successor executor: %v", err)
	}
	if running.AgentExecutionID != "execution-successor" {
		t.Fatalf("agent execution after successor registration = %q, want successor", running.AgentExecutionID)
	}

	for _, test := range []struct {
		name          string
		seedError     *models.LastAgentError
		expectedStamp string
	}{
		{name: "absent error"},
		{
			name:          "unchanged stamp",
			seedError:     &models.LastAgentError{Message: "old", OccurredAt: time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC), StampValue: "old-stamp"},
			expectedStamp: "old-stamp",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			taskID := "task-pg-session-lock-failure-" + test.name
			sessionID := "session-pg-session-lock-failure-" + test.name
			seedPostgresTaskSession(t, repo, taskID, sessionID)
			if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = ? WHERE id = ?`), models.TaskSessionStateStarting, sessionID); err != nil {
				t.Fatalf("seed starting state: %v", err)
			}
			if test.seedError != nil {
				if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, *test.seedError); err != nil {
					t.Fatalf("seed existing error: %v", err)
				}
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "execution-old", Status: models.ExecutorRunningStatusStarting,
			}); err != nil {
				t.Fatalf("seed old executor: %v", err)
			}

			holder, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin successor transaction: %v", err)
			}
			defer func() { _ = holder.Rollback() }()
			if _, err := holder.ExecContext(ctx, `SELECT id FROM task_sessions WHERE id = $1 FOR UPDATE`, sessionID); err != nil {
				t.Fatalf("lock session for successor: %v", err)
			}
			if _, err := holder.ExecContext(ctx, `UPDATE executors_running SET agent_execution_id = $1 WHERE session_id = $2`, "execution-successor", sessionID); err != nil {
				t.Fatalf("install successor execution: %v", err)
			}

			failureDone := make(chan struct {
				changed bool
				err     error
			}, 1)
			go func() {
				changed, _, err := repo.CommitBootstrapFailureIfCurrentExecution(
					ctx, taskID, sessionID, "execution-old", models.TaskSessionStateStarting, test.expectedStamp,
					models.LastAgentError{Message: "stale failure", OccurredAt: time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC), StampValue: "new-stamp"},
				)
				failureDone <- struct {
					changed bool
					err     error
				}{changed: changed, err: err}
			}()
			select {
			case result := <-failureDone:
				t.Fatalf("bootstrap failure completed while successor transaction held session lock: changed=%t err=%v", result.changed, result.err)
			case <-time.After(100 * time.Millisecond):
			}
			if err := holder.Commit(); err != nil {
				t.Fatalf("commit successor transaction: %v", err)
			}
			result := <-failureDone
			if result.err != nil {
				t.Fatalf("bootstrap failure commit: %v", result.err)
			}
			if result.changed {
				t.Fatal("stale bootstrap failure committed after successor registration")
			}
		})
	}
}

func TestPostgresSessionMetadataLaunchErrorCASUsesJSONB(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-session-launch-error", "Session launch error", `{"other_key":"keep me"}`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, metadata, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-pg-launch-error", "task-pg-session-launch-error", `{"other_key":"keep me"}`, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "session-pg-launch-error", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message:    "old error",
		OccurredAt: now,
		StampValue: "old-stamp",
	}); err != nil {
		t.Fatalf("set session metadata with postgres JSONB: %v", err)
	}
	stored, err := repo.SetSessionMetadataKeyIfStamp(ctx, "session-pg-launch-error", models.SessionMetaKeyLastAgentError, "old-stamp", models.LastAgentError{
		Message:    "new error",
		OccurredAt: now.Add(time.Minute),
		StampValue: "new-stamp",
	})
	if err != nil {
		t.Fatalf("stamped session metadata with postgres JSONB: %v", err)
	}
	if !stored {
		t.Fatal("postgres stamped session metadata write did not land")
	}

	session, err := repo.GetTaskSession(ctx, "session-pg-launch-error")
	if err != nil {
		t.Fatalf("load postgres session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Stamp() != "new-stamp" {
		t.Fatalf("postgres session error = %#v, want new-stamp", lastError)
	}
	if session.Metadata["other_key"] != "keep me" {
		t.Fatalf("session-level other_key = %#v, want preserved value", session.Metadata["other_key"])
	}
}
