package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListUnresolvedClarificationBundlesScansAggregateCreatedAt
// proves the PostgreSQL-specific scan branch of
// scanClarificationBundleRows: created_at is a MIN() aggregate, and the
// SQLite-only test run never exercises the isPostgres=true path documented
// there (spec M4, "schema replay alone is insufficient").
func TestPostgresListUnresolvedClarificationBundlesScansAggregateCreatedAt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-bundle", "")
	seedBundleSession(t, repo, "sess-pg-bundle", "task-pg-bundle")
	seedBundleTurn(t, repo, "turn-pg-bundle", "sess-pg-bundle", "task-pg-bundle")
	ts := time.Now().UTC().Truncate(time.Millisecond)
	insertClarificationMessage(t, repo, "msg-pg-bundle", "sess-pg-bundle", "task-pg-bundle", "turn-pg-bundle", "pending-pg-bundle", "q1", "pending", 0, ts)

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-pg-bundle" {
		t.Fatalf("bundles = %+v, want exactly pending-pg-bundle", page.Bundles)
	}
	if !page.Bundles[0].CreatedAt.Equal(ts) {
		t.Fatalf("CreatedAt = %v, want %v", page.Bundles[0].CreatedAt, ts)
	}
}

// TestPostgresListUnresolvedClarificationBundlesCursorPagination proves L9/D6
// cursor pagination against a real MIN() aggregate comparison on Postgres,
// where the bound time.Time cursor arg is compared to a jsonb-derived
// aggregate rather than a plain column.
func TestPostgresListUnresolvedClarificationBundlesCursorPagination(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-page", "")
	seedBundleSession(t, repo, "sess-pg-page", "task-pg-page")
	seedBundleTurn(t, repo, "turn-pg-page", "sess-pg-page", "task-pg-page")
	base := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"pending-pg-1", "pending-pg-2", "pending-pg-3"}
	for i, id := range ids {
		insertClarificationMessage(t, repo, "msg-"+id, "sess-pg-page", "task-pg-page", "turn-pg-page", id, "q1", "pending", 0, base.Add(time.Duration(i)*time.Second))
	}

	firstPage, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(firstPage.Bundles) != 2 || firstPage.Bundles[0].PendingID != "pending-pg-1" || firstPage.Bundles[1].PendingID != "pending-pg-2" {
		t.Fatalf("first page = %+v, want [pending-pg-1, pending-pg-2]", firstPage.Bundles)
	}
	if !firstPage.HasMore {
		t.Fatalf("first page HasMore = false, want true")
	}

	last := firstPage.Bundles[len(firstPage.Bundles)-1]
	opts := unscopedOpts(2)
	opts.CursorCreatedAt = last.CreatedAt
	opts.CursorPendingID = last.PendingID
	secondPage, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(secondPage.Bundles) != 1 || secondPage.Bundles[0].PendingID != "pending-pg-3" {
		t.Fatalf("second page = %+v, want exactly [pending-pg-3]", secondPage.Bundles)
	}
	if secondPage.HasMore {
		t.Fatalf("second page HasMore = true, want false")
	}
}
