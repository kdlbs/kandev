package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

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
