package sqlite

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryExternalOwnershipAndIntegerNormalization(t *testing.T) {
	for _, engine := range []struct {
		name     string
		postgres bool
	}{{name: "sqlite"}, {name: "postgres", postgres: true}} {
		t.Run(engine.name, func(t *testing.T) {
			var dbConn *sqlx.DB
			if engine.postgres {
				dbConn = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
				var version string
				require.NoError(t, dbConn.Get(&version, `SHOW server_version`))
				t.Logf("PostgreSQL %s", version)
			} else {
				raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "ownership.db"))
				require.NoError(t, err)
				dbConn = sqlx.NewDb(raw, "sqlite3")
				t.Cleanup(func() { _ = dbConn.Close() })
			}
			tasks, err := tasksqlite.NewWithDB(dbConn, dbConn, nil)
			require.NoError(t, err)
			repo, err := NewWithDB(dbConn, dbConn)
			if err != nil {
				t.Fatalf("initialize analytics schema: %v", err)
			}
			tasks.SetUsageHistoryRetainer(repo)
			now := time.Date(2026, time.October, 28, 12, 30, 0, 0, time.UTC)
			created := now.Add(-24 * time.Hour)
			require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "usage-ws", Name: "Usage history", OrgID: "org-trusted"}))
			task := &taskmodels.Task{ID: "usage-task", WorkspaceID: "usage-ws", Title: "Usage task", CreatedAt: created, UpdatedAt: created}
			require.NoError(t, tasks.CreateTask(t.Context(), task))
			require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{
				ID: "usage-session", TaskID: task.ID, QueueIncarnationID: "usage-incarnation-7", StartedAt: created,
				Metadata: map[string]any{"acp": map[string]any{"session_id": "usage-transcript"}},
			}))
			var taskIncarnation, sessionIncarnation string
			require.NoError(t, dbConn.QueryRowx(dbConn.Rebind(`SELECT task_incarnation_id,session_incarnation_id
				FROM usage_subjects WHERE task_id = ? AND session_id = ?`), task.ID, "usage-session").Scan(&taskIncarnation, &sessionIncarnation))
			tx, err := dbConn.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			for i, actor := range []string{"authenticated-actor-a", "authenticated-actor-b"} {
				require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
					ID: actor, Kind: usagehistory.BindingKindTurn, WorkspaceID: "usage-ws", OrganizationID: "org-trusted",
					TaskID: task.ID, TaskIncarnationID: taskIncarnation,
					SessionID: "usage-session", SessionIncarnationID: sessionIncarnation,
					ActorID: actor, ActorStatus: usagehistory.ActorStatusAttributed, ActorKind: usagehistory.ActorKindUser,
					ActorSource: "authenticated-turn", CapturedAt: now.Add(time.Duration(i+1) * time.Hour),
				}))
			}
			require.NoError(t, tx.Commit())
			maxInt := int64(math.MaxInt64)
			reasoningOutput := int64(17)
			measurements := []models.SessionUsageMeasurement{
				{
					WorkspaceID: "usage-ws", TaskID: "usage-task", SessionID: "usage-session", TranscriptID: "usage-transcript",
					ScopeID: "forged-scope", SubjectID: "forged-subject", UsageBindingID: "forged-binding",
					ActorID: "forged-user", ActorStatus: models.UsageAttributionAttributed, ActorKind: "user",
					Source: "plugin:collector", SourceRecordID: "collector-record-1", UsageIdentity: "work-1",
					Model: "provider/model-exact", Provider: "provider", AgentClient: "agent-cli",
					SourceVersion: "collector-v3", Revision: 1, PayloadDigest: "digest-1",
					ObservedAt: now, CollectedAt: now, InputTokens: &maxInt, TotalTokens: &maxInt,
					SourceTotalTokens: &maxInt, CacheReadTokens: models.Int64(9),
					ReasoningOutputTokens: &reasoningOutput, TokenSemanticsVersion: 4,
					CacheReadInsideInput: true, CostSubcents: &maxInt, Currency: "USD",
					CostBasis: models.UsageCostBasisReported, CostCoverage: models.UsageCoverageComplete,
				},
				{
					WorkspaceID: "usage-ws", TaskID: "usage-task", SessionID: "usage-session", TranscriptID: "usage-transcript",
					ScopeID: "forged-other-scope", SubjectID: "forged-other-subject", UsageBindingID: "forged-other-binding",
					ActorID: "forged-second-user", ActorStatus: models.UsageAttributionMixed, ActorKind: "user",
					Source: "plugin:collector", SourceRecordID: "collector-record-2", UsageIdentity: "work-2",
					Model: "provider/model-exact", Provider: "provider", Revision: 1, PayloadDigest: "digest-2",
					ObservedAt: now, CollectedAt: now.Add(3 * time.Hour), InputTokens: models.Int64(3), TotalTokens: models.Int64(3),
				},
			}
			results, err := repo.UpsertSessionUsage(context.Background(), measurements)
			if err != nil {
				t.Fatalf("UpsertSessionUsage: %v", err)
			}
			if len(results) != 2 || results[0].Status != models.UsageWriteApplied || results[1].Status != models.UsageWriteApplied {
				t.Fatalf("upsert results = %+v, want two applied records", results)
			}
			first := results[0].Measurement
			if first == nil || first.ScopeID == "" || first.ScopeID == "forged-scope" || first.SubjectID == "" ||
				first.SubjectID == "forged-subject" || first.UsageBindingID != "" || first.ActorID != "" ||
				first.ActorStatus != models.UsageAttributionUnknown || first.ActorKind != "unknown" {
				t.Fatalf("external actor/scope claims were not cleared and core-derived: %+v", first)
			}
			if first.SourceTotalTokens == nil || *first.SourceTotalTokens != math.MaxInt64 || first.InputTokens == nil ||
				*first.InputTokens != math.MaxInt64 || first.CostSubcents == nil || *first.CostSubcents != math.MaxInt64 ||
				first.ReasoningOutputTokens == nil || *first.ReasoningOutputTokens != reasoningOutput ||
				first.TokenSemanticsVersion != 4 || !first.CacheReadInsideInput {
				t.Fatalf("normalized category semantics or 64-bit values changed: %+v", first)
			}
			second := results[1].Measurement
			if second == nil || second.ActorID != "" || second.ActorStatus != models.UsageAttributionMixed || second.ActorKind != "unknown" {
				t.Fatalf("mixed aggregate actor evidence = %+v, want actorless mixed status", second)
			}
			var scope struct {
				WorkspaceID    string `db:"workspace_id"`
				OrganizationID string `db:"organization_id"`
			}
			if err := dbConn.Get(&scope, dbConn.Rebind(`SELECT workspace_id, organization_id FROM usage_scopes WHERE scope_id = ?`), first.ScopeID); err != nil {
				t.Fatalf("read core-derived scope: %v", err)
			}
			if scope.WorkspaceID != "usage-ws" || scope.OrganizationID != "org-trusted" {
				t.Fatalf("retained scope = %+v, want trusted workspace/org", scope)
			}
			var incarnation string
			if err := dbConn.Get(&incarnation, dbConn.Rebind(`SELECT session_incarnation_id FROM usage_subjects WHERE subject_id = ?`), first.SubjectID); err != nil {
				t.Fatalf("read retained session incarnation: %v", err)
			}
			if incarnation != "usage-incarnation-7" {
				t.Fatalf("retained session incarnation = %q, want usage-incarnation-7", incarnation)
			}
			var dirty int
			if err := dbConn.Get(&dirty, dbConn.Rebind(`SELECT COUNT(*) FROM usage_dirty_partitions WHERE scope_id = ? AND subject_id = ?`), first.ScopeID, first.SubjectID); err != nil {
				t.Fatalf("count durable dirty partitions: %v", err)
			}
			if dirty != 2 {
				t.Fatalf("dirty partition count = %d, want 2 for distinct token-semantics partitions", dirty)
			}
			_, err = repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{{
				WorkspaceID: "usage-ws", TaskID: "usage-task", Source: "native", SourceRecordID: "forged-native",
				UsageIdentity: "forged", ObservedAt: now,
			}})
			if !errors.Is(err, ErrInvalidSessionUsage) {
				t.Fatalf("public native upsert error = %v, want ErrInvalidSessionUsage", err)
			}
		})
	}
}

func TestUsageHistorySQLiteSchemaDoesNotDependOnLiveRows(t *testing.T) {
	dbConn := createTestDB(t)
	if _, err := NewWithDB(dbConn, dbConn); err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		rows, err := dbConn.Queryx(`PRAGMA foreign_key_list("` + table + `")`)
		if err != nil {
			t.Fatalf("inspect %s foreign keys: %v", table, err)
		}
		for rows.Next() {
			var id, sequence int
			var parent, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &sequence, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				_ = rows.Close()
				t.Fatalf("scan %s foreign key: %v", table, err)
			}
			_ = rows.Close()
			t.Fatalf("retained table %s still references live table %s", table, parent)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close %s foreign keys: %v", table, err)
		}
	}
}

func TestUsageHistorySQLiteMigratesAwayLiveForeignKeys(t *testing.T) {
	dbConn := createTestDB(t)
	now := time.Now().UTC()
	for _, statement := range []string{
		`INSERT INTO workspaces (id, name, org_id, created_at, updated_at) VALUES ('legacy-ws', 'Legacy', 'legacy-org', ?, ?)`,
		`INSERT INTO tasks (id, workspace_id, board_id, title, created_at, updated_at) VALUES ('legacy-task', 'legacy-ws', '', 'Legacy', ?, ?)`,
		`INSERT INTO task_sessions (id, task_id, queue_incarnation_id, agent_profile_id, state, started_at, updated_at) VALUES ('legacy-session', 'legacy-task', 'legacy-incarnation', 'profile', 'WAITING_FOR_INPUT', ?, ?)`,
	} {
		if _, err := dbConn.Exec(statement, now, now); err != nil {
			t.Fatalf("seed legacy ownership rows: %v", err)
		}
	}
	legacyUsageDDL := `
		CREATE TABLE session_usage_measurements (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
			transcript_id TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, source_record_id TEXT NOT NULL,
			usage_identity TEXT NOT NULL, model TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '',
			source_version TEXT NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0,
			payload_digest TEXT NOT NULL DEFAULT '', observed_at TIMESTAMP NOT NULL, collected_at TIMESTAMP NOT NULL,
			input_tokens BIGINT, output_tokens BIGINT, cache_read_tokens BIGINT, cache_write_tokens BIGINT,
			reasoning_tokens BIGINT, total_tokens BIGINT, turns BIGINT, cost_subcents BIGINT,
			currency TEXT NOT NULL DEFAULT '', cost_basis TEXT NOT NULL DEFAULT 'unknown',
			cost_coverage TEXT NOT NULL DEFAULT 'missing', coverage TEXT NOT NULL DEFAULT 'complete',
			source_timezone TEXT NOT NULL DEFAULT '', attribution_status TEXT NOT NULL DEFAULT 'attributed',
			coverage_start TIMESTAMP, coverage_end TIMESTAMP, source_date TEXT NOT NULL DEFAULT '',
			coverage_key TEXT NOT NULL DEFAULT '', estimated INTEGER NOT NULL DEFAULT 0, stale INTEGER NOT NULL DEFAULT 0,
			UNIQUE(workspace_id, source, source_record_id, usage_identity, model, provider)
		);
		CREATE TABLE session_usage_buckets (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
			transcript_id TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, source_record_id TEXT NOT NULL,
			usage_identity TEXT NOT NULL, model TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '',
			source_version TEXT NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0,
			payload_digest TEXT NOT NULL DEFAULT '', observed_at TIMESTAMP NOT NULL, collected_at TIMESTAMP NOT NULL,
			input_tokens BIGINT, output_tokens BIGINT, cache_read_tokens BIGINT, cache_write_tokens BIGINT,
			reasoning_tokens BIGINT, total_tokens BIGINT, turns BIGINT, cost_subcents BIGINT,
			currency TEXT NOT NULL DEFAULT '', cost_basis TEXT NOT NULL DEFAULT 'unknown',
			cost_coverage TEXT NOT NULL DEFAULT 'missing', coverage TEXT NOT NULL DEFAULT 'complete',
			source_timezone TEXT NOT NULL DEFAULT '', attribution_status TEXT NOT NULL DEFAULT 'attributed',
			coverage_start TIMESTAMP, coverage_end TIMESTAMP, source_date TEXT NOT NULL DEFAULT '',
			coverage_key TEXT NOT NULL, estimated INTEGER NOT NULL DEFAULT 0, stale INTEGER NOT NULL DEFAULT 0,
			UNIQUE(workspace_id, source, source_record_id, usage_identity, model, provider, coverage_key)
		);`
	if _, err := dbConn.Exec(legacyUsageDDL); err != nil {
		t.Fatalf("create legacy retained usage schema: %v", err)
	}
	if _, err := dbConn.Exec(`INSERT INTO session_usage_buckets (id, workspace_id, task_id, session_id, source, source_record_id,
		usage_identity, model, provider, observed_at, collected_at, coverage_key) VALUES
		('legacy-bucket', 'legacy-ws', 'legacy-task', 'legacy-session', 'plugin:old', 'record-1', 'work-1', 'model', 'provider', ?, ?, '2026-10-28')`, now, now); err != nil {
		t.Fatalf("insert legacy retained row: %v", err)
	}

	if _, err := NewWithDB(dbConn, dbConn); err != nil {
		t.Fatalf("migrate retained usage schema: %v", err)
	}
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		rows, err := dbConn.Queryx(`PRAGMA foreign_key_list("` + table + `")`)
		if err != nil {
			t.Fatalf("inspect migrated %s foreign keys: %v", table, err)
		}
		if rows.Next() {
			var id, sequence int
			var parent, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &sequence, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				_ = rows.Close()
				t.Fatalf("scan migrated %s foreign key: %v", table, err)
			}
			_ = rows.Close()
			t.Fatalf("migrated retained table %s still references %s", table, parent)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close migrated %s foreign key rows: %v", table, err)
		}
	}
	if _, err := dbConn.Exec(`DELETE FROM tasks WHERE id = 'legacy-task'`); err != nil {
		t.Fatalf("delete legacy task after schema migration: %v", err)
	}
	var count int
	if err := dbConn.Get(&count, `SELECT COUNT(*) FROM session_usage_buckets WHERE id = 'legacy-bucket' AND session_id = 'legacy-session'`); err != nil {
		t.Fatalf("read migrated retained row after task deletion: %v", err)
	}
	if count != 1 {
		t.Fatalf("legacy retained rows after task deletion = %d, want 1 with session identity preserved", count)
	}
}
