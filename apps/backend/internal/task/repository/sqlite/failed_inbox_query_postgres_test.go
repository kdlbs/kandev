package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPostgresListFailedInboxTasksScansDirectCompletedAtColumn proves the
// nullable completed_at column (a direct column reference through the
// correlated-subquery join, not a MIN() aggregate) round-trips through
// PostgreSQL's driver both when present and when the joined session is
// absent -- the SQLite-only run never exercises PostgreSQL's own NULL
// timestamp handling on this path.
func TestPostgresListFailedInboxTasksScansDirectCompletedAtColumn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg", Name: "ws-pg"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	seedFailedInboxTask(t, repo, "task-pg-resolved", "ws-pg", v1.TaskStateFailed, "")
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-pg-resolved", TaskID: "task-pg-resolved", IsPrimary: true,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(10)), ErrorMessage: "boom",
	})
	seedFailedInboxTask(t, repo, "task-pg-unresolved", "ws-pg", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws-pg", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %+v", page.Rows)
	}
	// Unresolvable sorts first.
	if page.Rows[0].TaskID != "task-pg-unresolved" || page.Rows[0].FailureInstant != nil {
		t.Fatalf("expected the unresolvable row first with a nil instant, got %+v", page.Rows[0])
	}
	if page.Rows[1].TaskID != "task-pg-resolved" || page.Rows[1].FailureInstant == nil {
		t.Fatalf("expected the resolved row second with a non-nil instant, got %+v", page.Rows[1])
	}
	if !page.Rows[1].FailureInstant.Equal(mustTime(10)) {
		t.Fatalf("FailureInstant = %v, want %v", page.Rows[1].FailureInstant, mustTime(10))
	}
}

// TestPostgresListFailedInboxTasksTieBreaksByteOrdered proves
// dialect.ByteOrderedText's explicit COLLATE "C" actually forces byte order
// on PostgreSQL for both the session-id and task-id tiebreaks
// (AC-UI-INBOX-FAILED-001.10a, .30a) -- SQLite's default BINARY collation
// would pass this same assertion even with the collation omitted, so only
// the PostgreSQL run can catch a locale-collation regression here.
func TestPostgresListFailedInboxTasksTieBreaksByteOrdered(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-tie", Name: "ws-pg-tie"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	seedFailedInboxTask(t, repo, "task-pg-b", "ws-pg-tie", v1.TaskStateFailed, "")
	seedFailedInboxTask(t, repo, "task-pg-a", "ws-pg-tie", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws-pg-tie", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[0].TaskID != "task-pg-a" || page.Rows[1].TaskID != "task-pg-b" {
		t.Fatalf("expected byte-ordered [task-pg-a, task-pg-b], got %+v", page.Rows)
	}
}
