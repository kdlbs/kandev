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
// on PostgreSQL for the task-id tiebreak (AC-UI-INBOX-FAILED-001.10a). The
// ids deliberately differ only in case: "B" (0x42) sorts before "a" (0x61)
// in byte/C-locale order, but a language-aware default collation typically
// compares letters primarily case-insensitively and would put "a" first --
// two same-case ids (e.g. "a"/"b") would pass this assertion identically
// whether or not COLLATE "C" is applied, which is exactly the weak fixture
// this replaced. See TestPostgresListFailedInboxTasksSessionTieBreaksByteOrdered
// for the sibling session-id coverage (AC .30a) this file previously lacked
// entirely.
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
	seedFailedInboxTask(t, repo, "task-pg-a", "ws-pg-tie", v1.TaskStateFailed, "")
	seedFailedInboxTask(t, repo, "task-pg-B", "ws-pg-tie", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws-pg-tie", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[0].TaskID != "task-pg-B" || page.Rows[1].TaskID != "task-pg-a" {
		t.Fatalf("expected byte-ordered [task-pg-B, task-pg-a], got %+v", page.Rows)
	}
}

// TestPostgresListFailedInboxTasksSessionTieBreaksByteOrdered proves the same
// COLLATE "C" behavior for the session-id tiebreak (AC-UI-INBOX-FAILED-001.30a)
// -- the sibling test above only ever exercised the task-id tiebreak, so a
// missing collation on the session-id comparison had zero PostgreSQL coverage.
// Mirrors the SQLite-only
// TestListFailedInboxTasks_SessionResolutionTieBreaksByLowestSessionID fixture
// shape (two non-primary sessions on the same task, tied on started_at), with
// case-differing ids for the same reason as above.
func TestPostgresListFailedInboxTasksSessionTieBreaksByteOrdered(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-sess-tie", Name: "ws-pg-sess-tie"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	seedFailedInboxTask(t, repo, "task-pg-sess-tie", "ws-pg-sess-tie", v1.TaskStateFailed, "")
	tie := mustTime(10)
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-pg-a", TaskID: "task-pg-sess-tie", IsPrimary: false,
		StartedAt: tie, CompletedAt: timePtr(mustTime(15)), ErrorMessage: "session a",
	})
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-pg-B", TaskID: "task-pg-sess-tie", IsPrimary: false,
		StartedAt: tie, CompletedAt: timePtr(mustTime(20)), ErrorMessage: "session B",
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws-pg-sess-tie", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Reason != "session B" {
		t.Fatalf("expected the tie broken toward the byte-ordered lower session id (sess-pg-B), got %+v", page.Rows)
	}
}
