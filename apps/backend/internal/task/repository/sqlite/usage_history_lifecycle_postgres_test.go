package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUsageHistoryPostgresTaskDeletePreservesLegacySources(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()
	taskRepo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task schema: %v", err)
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	taskRepo.SetUsageHistoryRetainer(analyticsRepo)
	ensureRetainedUsageMigration(t, db)
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "usage-lifecycle-pg-ws", Name: "Usage lifecycle", OrgID: "usage-lifecycle-pg-org"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "usage-lifecycle-pg-task", WorkspaceID: "usage-lifecycle-pg-ws", Title: "Lifecycle"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "usage-lifecycle-pg-session", TaskID: "usage-lifecycle-pg-task", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	event := newTestUsageEvent("usage-lifecycle-pg-native", "usage-lifecycle-pg-task", "usage-lifecycle-pg-session")
	if err := taskRepo.CreateTaskUsageEvent(ctx, event); err != nil {
		t.Fatalf("create native source event: %v", err)
	}
	seedPostgresLegacyUsage(t, db, "usage-lifecycle-pg-task", "usage-lifecycle-pg-session", "usage-lifecycle-pg-external")

	if err := taskRepo.DeleteTask(ctx, "usage-lifecycle-pg-task"); err != nil {
		t.Fatalf("delete task through lifecycle: %v", err)
	}
	var nativeCount int
	if err := db.Get(&nativeCount, db.Rebind(`SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`), event.UsageEventID); err != nil {
		t.Fatalf("count retained native source: %v", err)
	}
	if nativeCount != 1 {
		t.Fatalf("retained native rows after task deletion = %d, want 1", nativeCount)
	}
	var legacy struct {
		ScopeID   string `db:"scope_id"`
		SubjectID string `db:"subject_id"`
		ActorID   string `db:"actor_id"`
		Status    string `db:"actor_status"`
	}
	if err := db.Get(&legacy, db.Rebind(`SELECT scope_id, subject_id, actor_id, actor_status FROM session_usage_measurements WHERE source_record_id = ?`), "usage-lifecycle-pg-external"); err != nil {
		t.Fatalf("read retained external source: %v", err)
	}
	if legacy.ScopeID == "" || legacy.SubjectID == "" || legacy.ActorID != "" || legacy.Status != "unknown" {
		t.Fatalf("retained external identity = %+v, want captured scope and unknown actor", legacy)
	}
	var nativeSourceRows, activeLinks int
	if err := db.Get(&nativeSourceRows, db.Rebind(`SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = ?`), event.UsageEventID); err != nil {
		t.Fatalf("count native live source rows: %v", err)
	}
	if err := db.Get(&activeLinks, db.Rebind(`SELECT COUNT(*) FROM usage_live_links WHERE live_task_id = ?`), "usage-lifecycle-pg-task"); err != nil {
		t.Fatalf("count attached task links: %v", err)
	}
	if nativeSourceRows != 0 || activeLinks != 0 {
		t.Fatalf("live sources/links after deletion = %d/%d, want 0/0", nativeSourceRows, activeLinks)
	}
}

func TestUsageHistoryPostgresCapturedOrgErasureAfterWorkspaceTransfer(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()
	taskRepo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task schema: %v", err)
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	taskRepo.SetUsageHistoryRetainer(analyticsRepo)
	ensureRetainedUsageMigration(t, db)
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "usage-erase-pg-ws", Name: "Usage erasure", OrgID: "usage-erase-pg-old-org"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "usage-erase-pg-task", WorkspaceID: "usage-erase-pg-ws", Title: "Erasure"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "usage-erase-pg-session", TaskID: "usage-erase-pg-task", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	event := newTestUsageEvent("usage-erase-pg-native", "usage-erase-pg-task", "usage-erase-pg-session")
	if err := taskRepo.CreateTaskUsageEvent(ctx, event); err != nil {
		t.Fatalf("create native source event: %v", err)
	}
	if err := reportExternalUsage(ctx, analyticsRepo, "usage-erase-pg-ws", "usage-erase-pg-session", "usage-erase", "usage-erase-pg-external", "model", 19, time.Now().UTC()); err != nil {
		t.Fatalf("create external retained source: %v", err)
	}
	var capturedScopeID string
	if err := db.Get(&capturedScopeID, db.Rebind(`SELECT scope_id FROM usage_scopes WHERE organization_id = ?`), "usage-erase-pg-old-org"); err != nil {
		t.Fatalf("read captured organization scope: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE workspaces SET org_id = ? WHERE id = ?`), "usage-erase-pg-new-org", "usage-erase-pg-ws"); err != nil {
		t.Fatalf("transfer workspace: %v", err)
	}
	if err := analyticsRepo.BeginOrganizationUsageErasure(ctx, "usage-erase-pg-old-org"); err != nil {
		t.Fatalf("begin organization usage erasure: %v", err)
	}
	if err := analyticsRepo.EraseOrganizationUsage(ctx, "usage-erase-pg-old-org"); err != nil {
		t.Fatalf("erase captured organization usage: %v", err)
	}
	for _, table := range []string{"usage_subjects", "usage_execution_bindings", "usage_dirty_partitions", "usage_live_links", "session_usage_measurements", "session_usage_buckets"} {
		var count int
		if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`), capturedScopeID); err != nil {
			t.Fatalf("count erased captured scope rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("captured organization rows remain in %s: %d", table, count)
		}
	}
	var scopes, nativeSources, externalSources int
	if err := db.Get(&scopes, db.Rebind(`SELECT COUNT(*) FROM usage_scopes WHERE organization_id = ?`), "usage-erase-pg-old-org"); err != nil {
		t.Fatalf("count erased scopes: %v", err)
	}
	if err := db.Get(&nativeSources, db.Rebind(`SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = ?`), event.UsageEventID); err != nil {
		t.Fatalf("count erased native sources: %v", err)
	}
	if err := db.Get(&externalSources, db.Rebind(`SELECT COUNT(*) FROM session_usage_measurements WHERE source_record_id = ?`), "usage-erase-pg-external"); err != nil {
		t.Fatalf("count erased external sources: %v", err)
	}
	if scopes != 0 || nativeSources != 0 || externalSources != 0 {
		t.Fatalf("captured-org scope/native/external rows after erasure = %d/%d/%d, want zero", scopes, nativeSources, externalSources)
	}
}

func seedPostgresLegacyUsage(t *testing.T, db *sqlx.DB, taskID, sessionID, recordID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`INSERT INTO session_usage_measurements
		(id, workspace_id, task_id, session_id, source, source_record_id, usage_identity, model, provider,
		revision, payload_digest, observed_at, collected_at, input_tokens, total_tokens, coverage_key,
		actor_status, actor_kind)
		VALUES (?, 'usage-lifecycle-pg-ws', ?, ?, 'plugin:legacy', ?, 'legacy-work', 'model', 'provider',
		1, 'legacy-digest', ?, ?, 23, 23, '', 'unknown', 'unknown')`),
		"usage-measurement-"+recordID, taskID, sessionID, recordID, now, now); err != nil {
		t.Fatalf("seed PostgreSQL legacy usage: %v", err)
	}
}
