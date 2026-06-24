package linear

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestStore_IssueWatch_RepositoryBindingRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	w.RepositoryID = "repo-123"
	w.BaseBranch = "develop"
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RepositoryID != "repo-123" || got.BaseBranch != "develop" {
		t.Fatalf("repository binding lost on round-trip: repo=%q branch=%q", got.RepositoryID, got.BaseBranch)
	}

	// Update clears the binding (unbind path).
	got.RepositoryID = ""
	got.BaseBranch = ""
	if err := store.UpdateIssueWatch(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, err := store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if after.RepositoryID != "" || after.BaseBranch != "" {
		t.Fatalf("expected binding cleared, got repo=%q branch=%q", after.RepositoryID, after.BaseBranch)
	}
}

// TestStore_IssueWatch_DefaultsUnbound pins the backward-compat invariant: a
// watch created without a repository binding reads back as unbound (empty),
// preserving the repo-less behaviour.
func TestStore_IssueWatch_DefaultsUnbound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RepositoryID != "" || got.BaseBranch != "" {
		t.Fatalf("unbound watch should have empty binding, got repo=%q branch=%q", got.RepositoryID, got.BaseBranch)
	}
}

// preRepoIssueWatchSchema is the linear_issue_watches DDL from before the
// repository-binding columns were introduced (but after the max_inflight /
// last_error migrations). Used to exercise addIssueWatchRepositoryColumns on an
// older database.
const preRepoIssueWatchSchema = `
	CREATE TABLE linear_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`

func TestStore_IssueWatch_AddRepositoryColumns_Migration(t *testing.T) {
	ctx := context.Background()
	raw, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })

	// Seed an old-schema table with one existing row.
	if _, err := raw.Exec(preRepoIssueWatchSchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	now := time.Now().UTC()
	id := uuid.New().String()
	if _, err := raw.Exec(`INSERT INTO linear_issue_watches
		(id, workspace_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
		VALUES (?, 'ws-1', 'wf-1', 'step-1', '{}', ?, ?)`, id, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// NewStore runs initSchema, which must add the new columns idempotently.
	store, err := NewStore(raw, raw)
	if err != nil {
		t.Fatalf("NewStore on legacy table: %v", err)
	}

	got, err := store.GetIssueWatch(ctx, id)
	if err != nil {
		t.Fatalf("get migrated row: %v", err)
	}
	if got == nil {
		t.Fatal("expected migrated row, got nil")
	}
	if got.RepositoryID != "" || got.BaseBranch != "" {
		t.Fatalf("existing row should backfill to unbound, got repo=%q branch=%q", got.RepositoryID, got.BaseBranch)
	}

	// Idempotent: running initSchema again must not fail on duplicate columns.
	if err := store.initSchema(); err != nil {
		t.Fatalf("second initSchema (idempotency): %v", err)
	}
}
