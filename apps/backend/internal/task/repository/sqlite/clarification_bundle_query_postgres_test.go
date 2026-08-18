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

// TestPostgresListUnresolvedClarificationBundlesExcludesParentQuestion
// proves the parent-question exclusion (dialect.ExcludeTruthyMetadataPredicate)
// on the Postgres branch, where a jsonb boolean reads back as text ("true")
// rather than SQLite's integer 1 — the SQLite-only run never exercises that
// comparison.
func TestPostgresListUnresolvedClarificationBundlesExcludesParentQuestion(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-pq", "")
	seedBundleSession(t, repo, "sess-pg-pq", "task-pg-pq")
	seedBundleTurn(t, repo, "turn-pg-pq", "sess-pg-pq", "task-pg-pq")
	insertParentQuestionMessage(t, repo, "msg-pg-pq", "sess-pg-pq", "task-pg-pq", "turn-pg-pq", "pending-pg-pq", "q1", "pending", time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (parent-question bundle excluded)", page.Bundles)
	}
}

// TestPostgresListUnresolvedClarificationBundlesAbsentStatusCountsAsPending
// mirrors TestListUnresolvedClarificationBundles_AbsentStatusCountsAsPending
// (D4a conjunct 2) on Postgres: a message with no status metadata at all
// (jsonb key absent, not merely empty) must still count as pending. The
// SQLite-only run of this arm never exercises Postgres's jsonb "key missing"
// comparison, which reads differently from SQLite's absent-column semantics.
func TestPostgresListUnresolvedClarificationBundlesAbsentStatusCountsAsPending(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-absent", "")
	seedBundleSession(t, repo, "sess-pg-absent", "task-pg-absent")
	seedBundleTurn(t, repo, "turn-pg-absent", "sess-pg-absent", "task-pg-absent")
	insertClarificationMessage(t, repo, "msg-pg-absent", "sess-pg-absent", "task-pg-absent", "turn-pg-absent", "pending-pg-absent", "q1", "", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-pg-absent" {
		t.Fatalf("bundles = %+v, want exactly pending-pg-absent", page.Bundles)
	}
}

// TestPostgresListUnresolvedClarificationBundlesExcludesAllTerminalLegacyBundle
// mirrors TestListUnresolvedClarificationBundles_ExcludesAllTerminalLegacyBundle
// (D4a conjunct 2) on Postgres: a pre-upgrade bundle with no resolution row
// but every message already terminal must not resurface. The SQLite-only run
// of this arm never exercises the Postgres jsonb status-comparison branch.
func TestPostgresListUnresolvedClarificationBundlesExcludesAllTerminalLegacyBundle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-terminal", "")
	seedBundleSession(t, repo, "sess-pg-terminal", "task-pg-terminal")
	seedBundleTurn(t, repo, "turn-pg-terminal", "sess-pg-terminal", "task-pg-terminal")
	insertClarificationMessage(t, repo, "msg-pg-terminal", "sess-pg-terminal", "task-pg-terminal", "turn-pg-terminal", "pending-pg-terminal", "q1", "answered", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (all-terminal legacy bundle)", page.Bundles)
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
