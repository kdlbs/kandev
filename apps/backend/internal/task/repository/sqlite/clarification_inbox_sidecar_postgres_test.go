package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// seedAnswerablePostgresBundle mirrors seedAnswerableBundle but leaves
// WorkspaceID empty: CreateTask's workspace-row lock requires a real
// workspace row to exist under PostgreSQL's foreign-key enforcement, which
// SQLite does not require, and these tests use Unscoped:true options that
// never filter on workspace anyway.
func seedAnswerablePostgresBundle(t *testing.T, repo *Repository, n int) (pendingID string) {
	t.Helper()
	taskID := "task-pg-sidecar-" + itoa(n)
	sessionID := "session-pg-sidecar-" + itoa(n)
	turnID := "turn-pg-sidecar-" + itoa(n)
	pendingID = "pending-pg-sidecar-" + itoa(n)
	seedBundleTask(t, repo, taskID, "")
	seedBundleSession(t, repo, sessionID, taskID)
	seedBundleTurn(t, repo, turnID, sessionID, taskID)
	insertClarificationMessage(t, repo, "msg-pg-sidecar-"+itoa(n), sessionID, taskID, turnID, pendingID, "q1", "", 0, time.Now().UTC())
	return pendingID
}

// TestPostgresUpsertClarificationInboxSidecar_DismissedBundleExcludedFromMainList
// mirrors TestUpsertClarificationInboxSidecar_DismissedBundleExcludedFromMainList
// but on PostgreSQL, exercising clarificationSidecarJoin's LEFT JOIN against a
// jsonb-backed clarification_bundle_query (the SQLite-only run never proves the
// join and predicate compile and evaluate correctly against Postgres).
func TestPostgresUpsertClarificationInboxSidecar_DismissedBundleExcludedFromMainList(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	pendingID := seedAnswerablePostgresBundle(t, repo, 101)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", pendingID, models.ClarificationSidecarDismissed, nil, time.Now().UTC()); err != nil {
		t.Fatalf("upsert sidecar: %v", err)
	}

	opts := unscopedOpts(50)
	opts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: time.Now().UTC()}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("expected dismissed bundle excluded from main list, got %d bundles", len(page.Bundles))
	}

	otherOpts := unscopedOpts(50)
	otherOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-2", Now: time.Now().UTC()}
	otherPage, err := repo.ListUnresolvedClarificationBundles(ctx, otherOpts)
	if err != nil {
		t.Fatalf("list other user: %v", err)
	}
	if len(otherPage.Bundles) != 1 {
		t.Fatalf("expected bundle visible to a different operator, got %d bundles", len(otherPage.Bundles))
	}
}

// TestPostgresCountHiddenClarificationBundles_MatchesEnumerationAndReportsEarliestExpiry
// exercises CountHiddenClarificationBundles' dialect.IsPostgres branch
// (sql.NullTime scan of MIN(cs.snooze_until)) directly, which the SQLite-only
// suite never reaches.
func TestPostgresCountHiddenClarificationBundles_MatchesEnumerationAndReportsEarliestExpiry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	// Postgres `timestamptz` stores microsecond precision; Go's time.Now()
	// carries nanoseconds, so comparing the exact written value on read-back
	// needs the same truncation the column itself performs, or an unlucky
	// nanosecond remainder fails a numerically-correct round trip.
	now := time.Now().UTC().Truncate(time.Microsecond)

	dismissedID := seedAnswerablePostgresBundle(t, repo, 102)
	snoozedSoonID := seedAnswerablePostgresBundle(t, repo, 103)
	snoozedLaterID := seedAnswerablePostgresBundle(t, repo, 104)
	visibleID := seedAnswerablePostgresBundle(t, repo, 105)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", dismissedID, models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	soon := now.Add(time.Hour)
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedSoonID, models.ClarificationSidecarSnoozed, &soon, now); err != nil {
		t.Fatalf("snooze soon: %v", err)
	}
	later := now.Add(4 * time.Hour)
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedLaterID, models.ClarificationSidecarSnoozed, &later, now); err != nil {
		t.Fatalf("snooze later: %v", err)
	}

	countOpts := unscopedOpts(50)
	countOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Only: true, Now: now}
	summary, err := repo.CountHiddenClarificationBundles(ctx, countOpts)
	if err != nil {
		t.Fatalf("count hidden: %v", err)
	}
	if summary.HiddenCount != 3 {
		t.Fatalf("expected 3 hidden bundles, got %d", summary.HiddenCount)
	}
	if summary.NextSnoozeExpiry == nil || !summary.NextSnoozeExpiry.Equal(soon) {
		t.Fatalf("expected next snooze expiry %v, got %v", soon, summary.NextSnoozeExpiry)
	}

	mainOpts := unscopedOpts(50)
	mainOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: now}
	mainPage, err := repo.ListUnresolvedClarificationBundles(ctx, mainOpts)
	if err != nil {
		t.Fatalf("list main: %v", err)
	}
	if len(mainPage.Bundles) != 1 || mainPage.Bundles[0].PendingID != visibleID {
		t.Fatalf("expected exactly the un-hidden bundle in the main list, got %+v", mainPage.Bundles)
	}
}

// TestPostgresCountHiddenClarificationBundles_ZeroHiddenReportsNoExpiry proves
// the dialect.IsPostgres branch correctly reads a NULL MIN(cs.snooze_until)
// (via sql.NullTime, not sql.NullString) as "no expiry" rather than erroring
// on a type mismatch or defaulting to a zero-value time.
func TestPostgresCountHiddenClarificationBundles_ZeroHiddenReportsNoExpiry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	seedAnswerablePostgresBundle(t, repo, 106)

	countOpts := unscopedOpts(50)
	countOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Only: true, Now: now}
	summary, err := repo.CountHiddenClarificationBundles(ctx, countOpts)
	if err != nil {
		t.Fatalf("count hidden: %v", err)
	}
	if summary.HiddenCount != 0 {
		t.Fatalf("expected 0 hidden bundles, got %d", summary.HiddenCount)
	}
	if summary.NextSnoozeExpiry != nil {
		t.Fatalf("expected no next snooze expiry, got %v", summary.NextSnoozeExpiry)
	}
}

// TestPostgresGetClarificationInboxSidecarStates_ResolvesStateAndExpiry proves
// the sidecar-state lookup's IN (...) query and sql.NullTime scan work against
// Postgres.
func TestPostgresGetClarificationInboxSidecarStates_ResolvesStateAndExpiry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	// See the truncation note in TestPostgresCountHiddenClarificationBundles_
	// MatchesEnumerationAndReportsEarliestExpiry above.
	now := time.Now().UTC().Truncate(time.Microsecond)
	dismissedID := seedAnswerablePostgresBundle(t, repo, 107)
	snoozedID := seedAnswerablePostgresBundle(t, repo, 108)
	snoozeUntil := now.Add(time.Hour)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", dismissedID, models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedID, models.ClarificationSidecarSnoozed, &snoozeUntil, now); err != nil {
		t.Fatalf("snooze: %v", err)
	}

	states, err := repo.GetClarificationInboxSidecarStates(ctx, "user-1", []string{dismissedID, snoozedID, "unknown-pending-id"})
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 resolved states, got %d: %+v", len(states), states)
	}
	if states[dismissedID].State != models.ClarificationSidecarDismissed {
		t.Fatalf("dismissed state = %v", states[dismissedID].State)
	}
	if states[snoozedID].State != models.ClarificationSidecarSnoozed || states[snoozedID].SnoozeUntil == nil || !states[snoozedID].SnoozeUntil.Equal(snoozeUntil) {
		t.Fatalf("snoozed state = %+v, want snoozeUntil %v", states[snoozedID], snoozeUntil)
	}
}
