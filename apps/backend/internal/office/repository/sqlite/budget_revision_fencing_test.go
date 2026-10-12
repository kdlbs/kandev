package sqlite_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestBudgetPolicyRevisionMigration_LegacyRowsBackfillToOne covers
// AC-OFFICE-COSTS-003.1: a database created before the revision column
// exists gets it added as NOT NULL DEFAULT 1, an existing row is backfilled
// to 1, and a boot replay is a no-op (no new local error classifier is
// exercised beyond db.IsDuplicateColumnError, ADR 0027).
func TestBudgetPolicyRevisionMigration_LegacyRowsBackfillToOne(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Seed the pre-.3 office_budget_policies shape (no revision column) and
	// a legacy row, before the repo (and its migration) ever runs.
	if _, err := db.Exec(`
		CREATE TABLE office_budget_policies (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			scope_type TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			limit_subcents INTEGER NOT NULL,
			period TEXT NOT NULL,
			alert_threshold_pct INTEGER DEFAULT 80,
			action_on_exceed TEXT DEFAULT 'notify_only',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("seed legacy office_budget_policies: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO office_budget_policies (
			id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at
		) VALUES ('legacy-1', 'ws-legacy', 'workspace', 'ws-legacy', 1000, 'monthly', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}

	var revision int64
	if err := db.QueryRow(`SELECT revision FROM office_budget_policies WHERE id = 'legacy-1'`).Scan(&revision); err != nil {
		t.Fatalf("select revision on legacy row: %v", err)
	}
	if revision != 1 {
		t.Fatalf("legacy row revision = %d, want 1", revision)
	}

	// Boot replay must be a no-op, not an error.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
}

// legacyBudgetClaimsFixture builds an in-memory database still carrying
// PR #3287's three-column office_budget_claims shape (no revision, no FK
// dependency needed for these assertions), so the AC-OFFICE-COSTS-003.3a
// recreate actually has to run rather than being short-circuited by a fresh
// database's inline final-shape DDL.
func legacyBudgetClaimsFixture(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE office_budget_policies (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			scope_type TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			limit_subcents INTEGER NOT NULL,
			period TEXT NOT NULL,
			alert_threshold_pct INTEGER DEFAULT 80,
			action_on_exceed TEXT DEFAULT 'notify_only',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE office_budget_claims (
			policy_id TEXT NOT NULL,
			period_key TEXT NOT NULL,
			level TEXT NOT NULL,
			claimed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (policy_id, period_key, level),
			FOREIGN KEY (policy_id) REFERENCES office_budget_policies(id) ON DELETE CASCADE
		);
	`); err != nil {
		t.Fatalf("seed legacy office_budget_claims: %v", err)
	}
	return db
}

// TestRecreateBudgetClaims_OldShapeDatabase_GuardedAcrossBoots covers
// AC-OFFICE-COSTS-003.3a: booting against a database still carrying the
// pre-.3 three-column claim table runs the recreate (destroying whatever
// claims existed before that boot, which AC-OFFICE-COSTS-003.3 expressly
// permits); a claim written *after* that boot survives a second boot, which
// must probe, find revision present, and do nothing.
func TestRecreateBudgetClaims_OldShapeDatabase_GuardedAcrossBoots(t *testing.T) {
	db := legacyBudgetClaimsFixture(t)
	if _, err := db.Exec(`
		INSERT INTO office_budget_policies (
			id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at
		) VALUES ('p1', 'ws-1', 'workspace', 'ws-1', 1000, 'monthly', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	// A claim held before the cutover boot: the recreate is permitted to
	// destroy it (AC-OFFICE-COSTS-003.3's accepted first-run behavior).
	if _, err := db.Exec(`
		INSERT INTO office_budget_claims (policy_id, period_key, level, claimed_at)
		VALUES ('p1', 'lifetime', 'alert', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed pre-cutover claim: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first boot (cutover): %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM office_budget_claims`).Scan(&count); err != nil {
		t.Fatalf("count claims after cutover: %v", err)
	}
	if count != 0 {
		t.Fatalf("claims after cutover = %d, want 0 (pre-cutover claims are destroyed by the recreate)", count)
	}

	var revisionColExists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('office_budget_claims') WHERE name = 'revision'`).
		Scan(&revisionColExists); err != nil {
		t.Fatalf("probe revision column: %v", err)
	}
	if revisionColExists != 1 {
		t.Fatal("office_budget_claims.revision must exist after the cutover boot")
	}

	// A claim written after the cutover boot must survive a second boot.
	ctx := context.Background()
	if claimed, err := repo.Claim(ctx, "p1", "lifetime", "alert", 1); err != nil || !claimed {
		t.Fatalf("post-cutover claim: claimed=%v err=%v", claimed, err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM office_budget_claims`).Scan(&count); err != nil {
		t.Fatalf("count claims after replay: %v", err)
	}
	if count != 1 {
		t.Fatalf("claims after replay boot = %d, want 1 (the probe must no-op on a second boot)", count)
	}
}

// TestCreateBudgetPolicy_AssignsRevisionOne_IgnoringPresentedValue covers
// AC-OFFICE-COSTS-003.4: revision is always 1 on create, regardless of any
// value already set on the policy struct passed in, and the assigned value
// is written back so the caller sees what the row actually holds.
func TestCreateBudgetPolicy_AssignsRevisionOne_IgnoringPresentedValue(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()

	policy := createTestBudgetPolicy(t, repo, "ws-revision-create")
	policy.Revision = 777 // presented before a hypothetical re-create; must not survive.
	policy.ID = ""        // force a fresh insert
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if policy.Revision != 1 {
		t.Fatalf("revision returned to caller = %d, want 1", policy.Revision)
	}

	var stored int64
	if err := db.QueryRow(`SELECT revision FROM office_budget_policies WHERE id = ?`, policy.ID).Scan(&stored); err != nil {
		t.Fatalf("select stored revision: %v", err)
	}
	if stored != 1 {
		t.Fatalf("stored revision = %d, want 1", stored)
	}
}

// TestUpdateBudgetPolicy_NonexistentPolicy_ReturnsErrorNoCommit covers the
// AC-OFFICE-COSTS-003.5 tightening: updating a policy id that no longer
// exists rolls the transaction back and reports an error, a behavior change
// from #3287 where the update ignored its affected-row count and reported
// success.
func TestUpdateBudgetPolicy_NonexistentPolicy_ReturnsErrorNoCommit(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()

	policy := createTestBudgetPolicy(t, repo, "ws-revision-missing")
	if err := repo.DeleteBudgetPolicy(ctx, policy.ID); err != nil {
		t.Fatalf("delete policy: %v", err)
	}

	policy.LimitSubcents = 9999
	if err := repo.UpdateBudgetPolicy(ctx, policy); err == nil {
		t.Fatal("expected an error updating a policy id that no longer exists")
	}
}

// TestUpdateBudgetPolicy_ConcurrentUpdates_RevisionAdvancesByTwo covers
// concurrency case 6 of the forced-to-invent pass: two concurrent updates
// each bump by 1, and the final revision is r+2 with no increment lost —
// serialized by SQLite's single-writer connection, exactly as production
// runs (internal/db/pool.go).
func TestUpdateBudgetPolicy_ConcurrentUpdates_RevisionAdvancesByTwo(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-revision-concurrent")

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(limit int64) {
			defer wg.Done()
			p, err := repo.GetBudgetPolicy(ctx, policy.ID)
			if err != nil {
				t.Errorf("get policy: %v", err)
				return
			}
			p.LimitSubcents = limit
			if err := repo.UpdateBudgetPolicy(ctx, p); err != nil {
				t.Errorf("update policy: %v", err)
			}
		}(int64(2000 + i))
	}
	wg.Wait()

	var revision int64
	if err := db.QueryRow(`SELECT revision FROM office_budget_policies WHERE id = ?`, policy.ID).Scan(&revision); err != nil {
		t.Fatalf("select final revision: %v", err)
	}
	if revision != 3 {
		t.Fatalf("final revision = %d, want 3 (started at 1, two updates each +1)", revision)
	}
}

// TestClaim_RaceA_StaleRevisionRefused_FreshRevisionSucceeds covers
// AC-OFFICE-COSTS-003.8/.9/.15 (the verification.md "Race A regression"
// bullet): a claim presenting a superseded revision is refused as an
// ordinary miss (no error), and the slot for the new revision stays open so
// a claim at the current revision still wins.
func TestClaim_RaceA_StaleRevisionRefused_FreshRevisionSucceeds(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-race-a")
	staleRevision := policy.Revision // 1

	// Concurrent UpdateBudgetPolicy commits, bumping the stored revision to 2.
	policy.LimitSubcents = 2000
	if err := repo.UpdateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if policy.Revision != staleRevision+1 {
		t.Fatalf("revision after update = %d, want %d", policy.Revision, staleRevision+1)
	}

	// The stale evaluation's claim, fenced to the superseded revision, must
	// be refused with no error.
	claimed, err := repo.Claim(ctx, policy.ID, "lifetime", "alert", staleRevision)
	if err != nil {
		t.Fatalf("stale claim must not be a store error, got: %v", err)
	}
	if claimed {
		t.Fatal("a claim fenced to a superseded revision must not win")
	}

	// The slot for the new revision must still be open.
	claimed, err = repo.Claim(ctx, policy.ID, "lifetime", "alert", policy.Revision)
	if err != nil {
		t.Fatalf("fresh-revision claim: %v", err)
	}
	if !claimed {
		t.Fatal("a claim fenced to the current revision must win even after a stale claim was refused")
	}
}

// TestClaim_InvalidInput_FailsOpenNotSilently covers AC-OFFICE-COSTS-003.13:
// each of the four primary-key components, when invalid, is a claim-store
// error (never the silent miss AC-OFFICE-COSTS-003.9 reserves for a refused
// fence), so the guard must run before the fenced statement rather than as
// part of it.
func TestClaim_InvalidInput_FailsOpenNotSilently(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-invalid-input")

	cases := []struct {
		name             string
		policyID, period string
		level            string
		revision         int64
	}{
		{"zero revision", policy.ID, "lifetime", "alert", 0},
		{"negative revision", policy.ID, "lifetime", "alert", -1},
		{"empty policy id", "", "lifetime", "alert", policy.Revision},
		{"empty period key", policy.ID, "", "alert", policy.Revision},
		{"empty level", policy.ID, "lifetime", "", policy.Revision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claimed, err := repo.Claim(ctx, tc.policyID, tc.period, tc.level, tc.revision)
			if err == nil {
				t.Fatalf("expected a claim-store error for %s, got claimed=%v err=nil", tc.name, claimed)
			}
			if claimed {
				t.Fatalf("an errored claim attempt must report claimed=false, got true for %s", tc.name)
			}
		})
	}
}

// TestClaimExceeded_HappyPath_BothRowsExistAtSameRevision covers
// AC-OFFICE-COSTS-003.10/.11 on a healthy store: a successful ClaimExceeded
// call commits both the exceeded-level and alert-level companion rows at
// the same policy, period and revision.
func TestClaimExceeded_HappyPath_BothRowsExistAtSameRevision(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-exceeded-happy")

	claimed, err := repo.ClaimExceeded(ctx, policy.ID, "lifetime", policy.Revision)
	if err != nil {
		t.Fatalf("claim exceeded: %v", err)
	}
	if !claimed {
		t.Fatal("first exceeded claim should win")
	}

	for _, level := range []string{"exceeded", "alert"} {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ? AND period_key = 'lifetime' AND level = ? AND revision = ?`,
			policy.ID, level, policy.Revision,
		).Scan(&count); err != nil {
			t.Fatalf("count %s claim: %v", level, err)
		}
		if count != 1 {
			t.Fatalf("%s claim rows = %d, want 1", level, count)
		}
	}
}

// TestClaimExceeded_CompanionConflictIsPairSuccess covers the first
// verification.md "pair outcomes that are successes, not failures" case:
// with an alert-level claim already held, an over-limit evaluation's
// companion insert conflicts, but the pair still commits and the exceeded
// claim is won.
func TestClaimExceeded_CompanionConflictIsPairSuccess(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-companion-conflict")

	if claimed, err := repo.Claim(ctx, policy.ID, "lifetime", "alert", policy.Revision); err != nil || !claimed {
		t.Fatalf("seed alert claim: claimed=%v err=%v", claimed, err)
	}

	claimed, err := repo.ClaimExceeded(ctx, policy.ID, "lifetime", policy.Revision)
	if err != nil {
		t.Fatalf("claim exceeded: %v", err)
	}
	if !claimed {
		t.Fatal("exceeded claim must still win despite the companion insert conflicting")
	}
}

// TestClaimExceeded_AlreadyHeld_NoCompanionAttempted covers the second
// verification.md "pair outcomes" case: with an exceeded-level claim
// already held, a second over-limit evaluation's exceeded insert writes
// nothing, and no companion insert is attempted.
func TestClaimExceeded_AlreadyHeld_NoCompanionAttempted(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-exceeded-already-held")

	if claimed, err := repo.ClaimExceeded(ctx, policy.ID, "lifetime", policy.Revision); err != nil || !claimed {
		t.Fatalf("first exceeded claim: claimed=%v err=%v", claimed, err)
	}
	// Remove the companion so a second, incorrectly-attempted companion
	// insert would be observable.
	if _, err := db.Exec(
		`DELETE FROM office_budget_claims WHERE policy_id = ? AND level = 'alert'`, policy.ID,
	); err != nil {
		t.Fatalf("remove companion: %v", err)
	}

	claimed, err := repo.ClaimExceeded(ctx, policy.ID, "lifetime", policy.Revision)
	if err != nil {
		t.Fatalf("second exceeded claim: %v", err)
	}
	if claimed {
		t.Fatal("a second exceeded claim at an already-held (policy, period, revision) must not win")
	}

	var companionCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ? AND level = 'alert'`, policy.ID,
	).Scan(&companionCount); err != nil {
		t.Fatalf("count companion rows: %v", err)
	}
	if companionCount != 0 {
		t.Fatal("no companion insert must be attempted when the exceeded insert itself writes no row")
	}
}
