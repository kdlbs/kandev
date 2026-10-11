package sqlite

import (
	"fmt"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

type retentionStorageMeasurement struct {
	InboxRows       int64
	RawPayloadBytes int64
	CompactRows     int64
	GenericEffects  int64
	CanonicalRows   int64
	CanonicalBytes  int64
	PromptRows      int64
	PromptBytes     int64
	TerminalEffects int64
	WorkflowEffects int64
	CursorRows      int64
	DatabaseBytes   int64
	WALBytes        int64
	ReusableBytes   int64
	DeadTuples      int64
}

func measureRetentionStorage(t testing.TB, repo *Repository) retentionStorageMeasurement {
	t.Helper()
	var result retentionStorageMeasurement
	for _, sample := range []struct {
		query  string
		fields []any
	}{
		{`SELECT COUNT(*), COALESCE(SUM(LENGTH(payload)),0), COALESCE(SUM(CASE WHEN payload_pruned_at IS NOT NULL THEN 1 ELSE 0 END),0) FROM agent_delivery_inbox`, []any{&result.InboxRows, &result.RawPayloadBytes, &result.CompactRows}},
		{`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type = 'agent_delivery.event'`, []any{&result.GenericEffects}},
		{`SELECT COUNT(*), COALESCE(SUM(LENGTH(content)),0) FROM task_session_messages`, []any{&result.CanonicalRows, &result.CanonicalBytes}},
		{`SELECT COUNT(*), COALESCE(SUM(LENGTH(payload)),0) FROM agent_delivery_submissions`, []any{&result.PromptRows, &result.PromptBytes}},
		{`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type = 'agent_delivery.terminal_settlement'`, []any{&result.TerminalEffects}},
		{`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type = 'workflow.on_turn_complete'`, []any{&result.WorkflowEffects}},
		{`SELECT COUNT(*) FROM agent_delivery_cursors`, []any{&result.CursorRows}},
	} {
		if err := repo.db.QueryRow(sample.query).Scan(sample.fields...); err != nil {
			t.Fatal(err)
		}
	}
	if repo.db.DriverName() == "pgx" {
		measureRetentionPostgresStorage(t, repo, &result)
	} else {
		measureRetentionSQLiteStorage(t, repo, &result)
	}
	return result
}

func measureRetentionPostgresStorage(t testing.TB, repo *Repository, result *retentionStorageMeasurement) {
	t.Helper()
	if err := repo.db.QueryRow(`SELECT COALESCE(SUM(pg_total_relation_size(c.oid)),0) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = current_schema() AND c.relkind = 'r'`).Scan(&result.DatabaseBytes); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.QueryRow(`SELECT COALESCE(SUM(n_dead_tup),0) FROM pg_stat_user_tables WHERE schemaname = current_schema()`).Scan(&result.DeadTuples); err != nil {
		t.Fatal(err)
	}
	// Server WAL is shared with independent schemas and cannot be attributed to this fixture.
	result.WALBytes = -1
	result.ReusableBytes = -1
}

func measureRetentionSQLiteStorage(t testing.TB, repo *Repository, result *retentionStorageMeasurement) {
	t.Helper()
	var pageCount, pageSize, freePages int64
	for _, sample := range []struct {
		query string
		field *int64
	}{
		{`PRAGMA page_count`, &pageCount}, {`PRAGMA page_size`, &pageSize}, {`PRAGMA freelist_count`, &freePages},
	} {
		if err := repo.db.QueryRow(sample.query).Scan(sample.field); err != nil {
			t.Fatal(err)
		}
	}
	result.DatabaseBytes = pageCount * pageSize
	result.ReusableBytes = freePages * pageSize
	result.DeadTuples = -1
	var sequence int
	var name, path string
	if err := repo.db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + "-wal"); err == nil {
		result.WALBytes = info.Size()
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func retentionStorageMeasurementLabel(engine string, payloadSize int, phase string) string {
	return fmt.Sprintf("engine=%s payload_bytes=%d phase=%s", engine, payloadSize, phase)
}

func reopenRetentionStorageRepository(t *testing.T, repo *Repository) *Repository {
	t.Helper()
	var path, schema string
	postgres := repo.db.DriverName() == "pgx"
	if postgres {
		if err := repo.db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
			t.Fatal(err)
		}
	} else {
		var sequence int
		var name string
		if err := repo.db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
	}
	clock := repo.clockNow
	if err := repo.db.Close(); err != nil {
		t.Fatal(err)
	}
	var writer *sqlx.DB
	if postgres {
		dsn := testutil.PostgresDSNWithSearchPath(t, testutil.PostgresDSNFromEnv(t), schema)
		raw, err := db.OpenPostgres(dsn, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		writer = sqlx.NewDb(raw, "pgx")
		t.Cleanup(func() { _, _ = writer.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`); _ = writer.Close() })
	} else {
		raw, err := db.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		writer = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = writer.Close() })
	}
	reopened, err := NewWithDB(writer, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	reopened.clockNow = clock
	return reopened
}

func compactRetentionStorageFixture(t *testing.T, repo *Repository) {
	t.Helper()
	if repo.db.DriverName() == "pgx" {
		for _, query := range []string{`VACUUM (ANALYZE) agent_delivery_inbox`, `VACUUM (ANALYZE) agent_delivery_effects`} {
			if _, err := repo.db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
	} else if _, err := repo.db.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
}
