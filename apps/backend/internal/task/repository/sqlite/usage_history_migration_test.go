package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUsageHistoryBackfillResumesDurableKeysetsAndRecordsUnknownGaps(t *testing.T) {
	testUsageHistoryBackfillResumesDurableKeysetsAndRecordsUnknownGaps(t, newUsageEventsTestRepo(t))
}

func TestUsageHistoryPostgresBackfillResumesDurableKeysetsAndRecordsUnknownGaps(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	var version string
	if err := db.Get(&version, `SHOW server_version`); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	t.Logf("PostgreSQL %s", version)
	taskRepo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	testUsageHistoryBackfillResumesDurableKeysetsAndRecordsUnknownGaps(t, taskRepo)
}

func testUsageHistoryBackfillResumesDurableKeysetsAndRecordsUnknownGaps(t *testing.T, taskRepo *Repository) {
	t.Helper()
	ctx := context.Background()
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Usage history", OrgID: "org-migration"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	firstAnalytics, err := analyticssqlite.NewWithDB(taskRepo.db, taskRepo.db)
	if err != nil {
		t.Fatalf("initialize analytics repository: %v", err)
	}
	taskRepo.SetUsageHistoryRetainer(firstAnalytics)
	createUsageEventsTestTask(t, taskRepo, "task-history-migrate")
	createUsageEventsTestSession(t, taskRepo, "session-history-migrate", "task-history-migrate")
	assertUsageHistoryMigrationBirth(t, taskRepo)
	insertUsageEventRow(t, taskRepo, "legacy-migrate-native", "task-history-migrate", "session-history-migrate")
	var retainedBeforeBackfill int
	if err := taskRepo.db.Get(&retainedBeforeBackfill, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = 'legacy-migrate-native'`); err != nil {
		t.Fatalf("count native copies before backfill: %v", err)
	}
	if retainedBeforeBackfill != 0 {
		t.Fatalf("native copies before backfill = %d, want 0", retainedBeforeBackfill)
	}
	seedLegacyExternalUsage(t, taskRepo, "task-history-migrate", "session-history-migrate", "legacy-migrate-external")
	seedOrphanExternalUsage(t, taskRepo, "legacy-migrate-orphan")
	done, err := firstAnalytics.BackfillUsageHistory(ctx, 1)
	if err != nil {
		t.Fatalf("first source-adoption page: %v", err)
	}
	if done {
		t.Fatal("first one-row adoption page reported the full migration complete")
	}
	var nativeRetained int
	if err := taskRepo.db.Get(&nativeRetained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = 'legacy-migrate-native'`); err != nil {
		t.Fatalf("count native copies before adoption completes: %v", err)
	}
	if nativeRetained != 0 {
		t.Fatalf("native copies before adoption completed = %d, want 0", nativeRetained)
	}
	done, err = firstAnalytics.BackfillUsageHistory(ctx, 1)
	if err != nil {
		t.Fatalf("first native migration page: %v", err)
	}
	if done {
		t.Fatal("first one-row native page reported the full migration complete")
	}
	if err := taskRepo.db.Get(&nativeRetained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = 'legacy-migrate-native'`); err != nil {
		t.Fatalf("count first native copy: %v", err)
	}
	if nativeRetained != 1 {
		t.Fatalf("first native page retained %d rows, want 1", nativeRetained)
	}

	// Reconstructing the analytics repository models a process restart. The
	// migration cursor and its high water live in the database, not this value.
	secondAnalytics, err := analyticssqlite.NewWithDB(taskRepo.db, taskRepo.db)
	if err != nil {
		t.Fatalf("restart analytics repository: %v", err)
	}
	for page := 0; page < 10 && !done; page++ {
		done, err = secondAnalytics.BackfillUsageHistory(ctx, 1)
		if err != nil {
			t.Fatalf("migration page %d after restart: %v", page, err)
		}
	}
	if !done {
		var state []struct {
			Source   string `db:"source_key"`
			High     string `db:"high_water"`
			Last     string `db:"last_key"`
			Complete int    `db:"complete"`
		}
		if err := taskRepo.db.Select(&state, `SELECT source_key, high_water, last_key, complete FROM usage_history_migration_state ORDER BY source_key`); err != nil {
			t.Fatal(err)
		}
		var unindexed int
		if err := taskRepo.db.Get(&unindexed, `SELECT COUNT(*) FROM session_usage_measurements WHERE scope_id <> '' AND subject_id <> '' AND selection_partition = ''`); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("bounded migration did not finish within seeded source pages: state=%+v unindexed=%d", state, unindexed)
	}
	if done, err := secondAnalytics.BackfillUsageHistory(ctx, 1); err != nil || !done {
		t.Fatalf("repeat completed migration = (%v, %v), want (true, nil)", done, err)
	}

	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		var duplicates int
		if err := taskRepo.db.Get(&duplicates, `SELECT COUNT(*) FROM `+table+` WHERE source_record_id = 'legacy-migrate-external'`); err != nil {
			t.Fatalf("count migrated rows in %s: %v", table, err)
		}
		if duplicates != 1 {
			t.Fatalf("migrated rows in %s = %d, want exactly one", table, duplicates)
		}
	}
	var orphanCopies, gaps int
	if err := taskRepo.db.Get(&orphanCopies, `SELECT COUNT(*) FROM session_usage_measurements WHERE source_record_id = 'legacy-migrate-orphan' AND scope_id <> ''`); err != nil {
		t.Fatalf("count orphan retained rows: %v", err)
	}
	if err := taskRepo.db.Get(&gaps, `SELECT COUNT(*) FROM usage_history_migration_gaps WHERE source_key = 'session_usage_measurements' AND record_key = 'measurement-legacy-migrate-orphan'`); err != nil {
		t.Fatalf("count unknown history gaps: %v", err)
	}
	if orphanCopies != 0 || gaps != 1 {
		t.Fatalf("orphan copies=%d gap records=%d, want no admission and one disclosed gap", orphanCopies, gaps)
	}
	var actorStatus string
	if err := taskRepo.db.Get(&actorStatus, `SELECT actor_status FROM session_usage_buckets WHERE source_record_id = 'legacy-migrate-native'`); err != nil {
		t.Fatalf("read migrated native actor status: %v", err)
	}
	if actorStatus != "unknown" {
		t.Fatalf("migrated legacy actor status = %q, want unknown", actorStatus)
	}
}

func assertUsageHistoryMigrationBirth(t *testing.T, repo *Repository) {
	t.Helper()
	var persisted struct {
		TaskCreatedAt        time.Time `db:"task_created_at"`
		SessionIncarnationID string    `db:"session_incarnation_id"`
	}
	if err := repo.db.Get(&persisted, `SELECT t.created_at AS task_created_at,
		COALESCE(NULLIF(s.queue_incarnation_id, ''), s.id) AS session_incarnation_id
		FROM tasks t JOIN task_sessions s ON s.task_id = t.id
		WHERE t.id = 'task-history-migrate' AND s.id = 'session-history-migrate'`); err != nil {
		t.Fatalf("read persisted migration identities: %v", err)
	}
	var births int
	if err := repo.db.Get(&births, repo.db.Rebind(`SELECT COUNT(*) FROM usage_collection_bindings b
		JOIN usage_scopes s ON s.scope_id = b.scope_id
		WHERE b.task_id = 'task-history-migrate' AND b.session_id = 'session-history-migrate'
		AND b.task_incarnation_id = ? AND b.session_incarnation_id = ? AND b.source_scope_status = 'eligible'
		AND s.workspace_id = 'ws-1' AND s.organization_id = 'org-migration'`),
		persisted.TaskCreatedAt.UTC().Format(time.RFC3339Nano), persisted.SessionIncarnationID); err != nil {
		t.Fatalf("read captured migration birth: %v", err)
	}
	if births != 1 {
		t.Fatalf("captured migration births for persisted identities = %d, want 1", births)
	}
}

func seedOrphanExternalUsage(t *testing.T, repo *Repository, recordID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO session_usage_measurements
		(id, workspace_id, task_id, source, source_record_id, usage_identity, model, provider,
		revision, payload_digest, observed_at, collected_at, input_tokens, total_tokens, coverage_key,
		actor_status, actor_kind)
		VALUES (?, 'ws-1', 'missing-task-legacy', 'plugin:legacy', ?, 'orphan-work', 'model', 'provider',
		1, 'orphan-digest', ?, ?, 9, 9, '', 'unknown', 'unknown')`),
		"measurement-"+recordID, recordID, now, now)
	if err != nil {
		t.Fatalf("seed orphan external usage: %v", err)
	}
}

func TestUsageHistoryOrganizationErasureUsesCapturedOrganizationAfterWorkspaceTransfer(t *testing.T) {
	ctx := context.Background()
	taskRepo := newUsageEventsTestRepo(t)
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Usage history", OrgID: "org-captured"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	createUsageEventsTestTask(t, taskRepo, "task-history-erase")
	createUsageEventsTestSession(t, taskRepo, "session-history-erase", "task-history-erase")
	analyticsRepo, err := analyticssqlite.NewWithDB(taskRepo.db, taskRepo.db)
	if err != nil {
		t.Fatalf("initialize analytics repository: %v", err)
	}
	taskRepo.SetUsageHistoryRetainer(analyticsRepo)
	native := newTestUsageEvent("native-before-org-transfer", "task-history-erase", "session-history-erase")
	if err := taskRepo.CreateTaskUsageEvent(ctx, native); err != nil {
		t.Fatalf("create retained native usage: %v", err)
	}
	seedLegacyExternalUsage(t, taskRepo, "task-history-erase", "session-history-erase", "external-before-org-transfer")
	done := false
	for pages := 0; pages < 4 && !done; pages++ {
		done, err = analyticsRepo.BackfillUsageHistory(ctx, 200)
		if err != nil {
			t.Fatalf("backfill legacy external scope before transfer: %v", err)
		}
	}
	if !done {
		t.Fatal("legacy external backfill did not finish before organization transfer")
	}
	var capturedGap int
	if err := taskRepo.db.Get(&capturedGap, `SELECT COUNT(*) FROM usage_history_migration_gaps WHERE source_key='native'
		AND task_id='task-history-erase' AND organization_id='org-captured'`); err != nil || capturedGap == 0 {
		t.Fatalf("unproven native source must keep captured erasure evidence: gaps=%d err=%v", capturedGap, err)
	}
	var capturedScopeID string
	if err := taskRepo.db.Get(&capturedScopeID, `SELECT scope_id FROM usage_scopes WHERE organization_id = 'org-captured'`); err != nil {
		t.Fatalf("read captured organization scope before transfer: %v", err)
	}
	if _, err := taskRepo.db.Exec(`UPDATE workspaces SET org_id = 'org-current' WHERE id = 'ws-1'`); err != nil {
		t.Fatalf("transfer workspace: %v", err)
	}
	if err := analyticsRepo.BeginOrganizationUsageErasure(ctx, "org-captured"); err != nil {
		t.Fatalf("begin captured organization erasure: %v", err)
	}
	fencedTx, err := taskRepo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin fenced binding attempt: %v", err)
	}
	fenceErr := analyticsRepo.AdmitBindingTx(ctx, fencedTx, usagehistory.Binding{
		ID: "late-org-binding", Kind: usagehistory.BindingKindExecution,
		WorkspaceID: "ws-1", OrganizationID: "org-captured", TaskID: "task-history-erase",
		TaskIncarnationID: "test-captured-incarnation",
		ActorStatus:       usagehistory.ActorStatusUnknown, ActorKind: usagehistory.ActorKindUnknown,
		ActorSource: "legacy-test", CapturedAt: time.Now().UTC(),
	})
	_ = fencedTx.Rollback()
	if fenceErr == nil || !strings.Contains(fenceErr.Error(), "usage scope erasure is in progress or complete") {
		t.Fatalf("binding admission during captured-org erasure = %v, want erasure fence", fenceErr)
	}
	if err := analyticsRepo.EraseOrganizationUsage(ctx, "org-captured"); err != nil {
		t.Fatalf("erase captured organization usage after transfer: %v", err)
	}
	for _, table := range []string{
		"usage_subjects", "usage_execution_bindings", "usage_dirty_partitions", "usage_live_links",
		"session_usage_measurements", "session_usage_buckets",
	} {
		var count int
		if err := taskRepo.db.Get(&count, `SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`, capturedScopeID); err != nil {
			t.Fatalf("count lifecycle rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("captured organization rows remain in %s: %d", table, count)
		}
	}
	for _, check := range []struct {
		table string
		where string
		args  []any
	}{
		{table: "usage_scopes", where: "organization_id = ?", args: []any{"org-captured"}},
		{table: "usage_preservation_fences", where: "organization_id = ?", args: []any{"org-captured"}},
		{table: "usage_history_migration_gaps", where: "organization_id = ?", args: []any{"org-captured"}},
	} {
		var count int
		if err := taskRepo.db.Get(&count, `SELECT COUNT(*) FROM `+check.table+` WHERE `+check.where, check.args...); err != nil {
			t.Fatalf("count captured organization rows in %s: %v", check.table, err)
		}
		if count != 0 {
			t.Fatalf("captured organization rows remain in %s: %d", check.table, count)
		}
	}
	var fenceState string
	var fenceGeneration int64
	if err := taskRepo.db.QueryRowx(`SELECT state, generation FROM usage_scope_fences WHERE fence_kind = 'organization' AND fence_id = 'org-captured'`).Scan(&fenceState, &fenceGeneration); err != nil {
		t.Fatalf("read organization erasure fence: %v", err)
	}
	if fenceState != "erased" || fenceGeneration < 2 {
		t.Fatalf("organization erasure fence = (%q, %d), want erased with advanced generation", fenceState, fenceGeneration)
	}
	var nativeRows int
	if err := taskRepo.db.Get(&nativeRows, `SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = 'native-before-org-transfer'`); err != nil {
		t.Fatalf("count native source after erasure: %v", err)
	}
	if nativeRows != 0 {
		t.Fatalf("native source rows after captured organization erasure = %d, want 0", nativeRows)
	}

	// E2E reset uses the workspace fence but reopens the workspace after all
	// usage subjects and side tables are cleared for its next fixture.
	newUsage := newTestUsageEvent("native-after-org-transfer", "task-history-erase", "session-history-erase")
	if err := taskRepo.CreateTaskUsageEvent(ctx, newUsage); err != nil {
		t.Fatalf("create new-org usage after erasure: %v", err)
	}
	if err := analyticsRepo.ResetWorkspaceUsageHistory(ctx, "ws-1"); err != nil {
		t.Fatalf("reset workspace usage history: %v", err)
	}
	var workspaceState string
	if err := taskRepo.db.Get(&workspaceState, `SELECT state FROM usage_scope_fences WHERE fence_kind = 'workspace' AND fence_id = 'ws-1'`); err != nil {
		t.Fatalf("read workspace reset fence: %v", err)
	}
	if workspaceState != "active" {
		t.Fatalf("workspace usage fence after reset = %q, want active", workspaceState)
	}
	if err := taskRepo.db.Get(&nativeRows, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native'`); err != nil || nativeRows != 0 {
		t.Fatalf("native retained rows after E2E reset = %d, err=%v; want zero", nativeRows, err)
	}
	if err := taskRepo.db.Get(&nativeRows, `SELECT COUNT(*) FROM task_usage_events`); err != nil || nativeRows != 0 {
		t.Fatalf("native ledger rows after E2E reset = %d, err=%v; want zero", nativeRows, err)
	}
}
