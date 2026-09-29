package coordinator

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/testutil"
)

const phase1DDL = `
	CREATE TABLE coordinators (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL, executor_profile_id TEXT NOT NULL,
		context TEXT NOT NULL DEFAULT '', conversation_task_id TEXT,
		created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL%s
	);
	CREATE TABLE coordinator_proposals (
		id TEXT PRIMARY KEY, coordinator_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
		status TEXT NOT NULL, spec_json TEXT NOT NULL, final_spec_json TEXT,
		claimed_at DATETIME, claim_token TEXT, task_id TEXT, error TEXT,
		reject_reason TEXT, decided_by TEXT,
		created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	);
	CREATE TABLE coordinator_stalls (
		task_id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, stalled_for_ms INTEGER NOT NULL,
		last_event_at DATETIME NOT NULL, detected_at DATETIME NOT NULL
	);
`

// tableColumns returns "name type" pairs of a table, dialect-neutral enough to
// compare two databases of the same dialect.
func tableColumns(t *testing.T, conn *sqlx.DB, table string) []string {
	t.Helper()
	query := `SELECT name || ' ' || type || ' ' || "notnull" || ' ' || COALESCE(dflt_value, '') FROM pragma_table_info('` + table + `')`
	args := []any{}
	if dialect.IsPostgres(conn.DriverName()) {
		query = conn.Rebind(`SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '') FROM information_schema.columns WHERE table_name = ? AND table_schema = current_schema()`)
		args = []any{table}
	}
	rows, err := conn.Queryx(query, args...)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}

func endSchema(t *testing.T, conn *sqlx.DB) string {
	t.Helper()
	var b strings.Builder
	for _, table := range []string{"coordinators", "coordinator_proposals", "coordinator_watches", "coordinator_activity", "coordinator_standing_orders", "coordinator_goals"} {
		b.WriteString(table + ": " + strings.Join(tableColumns(t, conn, table), "|") + "\n")
	}
	return b.String()
}

func openSQLitePool(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return sqlx.NewDb(conn, "sqlite3")
}

func upgradeFromPhase1(t *testing.T, conn *sqlx.DB, withConfigRevision bool) {
	t.Helper()
	extra := ""
	if withConfigRevision {
		extra = ", config_revision INTEGER NOT NULL DEFAULT 0"
	}
	ddl := dialect.MustRenderSchema(conn.DriverName(), strings.Replace(phase1DDL, "%s", extra, 1))
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("phase-1 ddl: %v", err)
	}
	if _, err := conn.Exec(conn.Rebind(`INSERT INTO coordinators (id, workspace_id, name, agent_profile_id, executor_profile_id, context, created_at, updated_at) VALUES ('c1','w1','n','a','e','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := conn.Exec(conn.Rebind(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at) VALUES ('p1','c1','w1','pending','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)); err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
	if _, err := NewStore(conn, conn); err != nil {
		t.Fatalf("NewStore over phase-1 schema: %v", err)
	}
	if _, err := NewStore(conn, conn); err != nil {
		t.Fatalf("NewStore replay: %v", err)
	}
}

func assertUpgradedDefaults(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	var scope string
	var revision int
	var policy *string
	if err := conn.QueryRow(`SELECT watch_scope, policy_revision, policy_json FROM coordinators WHERE id = 'c1'`).Scan(&scope, &revision, &policy); err != nil {
		t.Fatalf("read coordinator: %v", err)
	}
	if scope != "all" || revision != 0 || policy != nil {
		t.Fatalf("defaults = %q %d %v", scope, revision, policy)
	}
	var kind, ids string
	var starts bool
	if err := conn.QueryRow(`SELECT kind, standing_order_ids, starts_agent FROM coordinator_proposals WHERE id = 'p1'`).Scan(&kind, &ids, &starts); err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	if kind != "create_task" || ids != "[]" || starts {
		t.Fatalf("proposal defaults = %q %q %v", kind, ids, starts)
	}
}

func TestPhase2Schema_UpgradeFromPhase1_SQLite(t *testing.T) {
	var schemas []string
	for _, withRevision := range []bool{false, true} {
		conn := openSQLitePool(t)
		upgradeFromPhase1(t, conn, withRevision)
		assertUpgradedDefaults(t, conn)
		schemas = append(schemas, endSchema(t, conn))
	}
	fresh := newTestStore(t)
	freshSchema := endSchema(t, fresh.db)
	// config_revision is part of the base schema, so an upgrade converges on the
	// fresh schema whether or not the column was already present.
	if !strings.Contains(schemas[0], "policy_revision") || schemas[0] != freshSchema {
		t.Fatalf("upgraded schema differs from fresh:\n%s\nvs\n%s", schemas[0], freshSchema)
	}
	if schemas[1] != freshSchema {
		t.Fatalf("upgrade with config_revision differs from fresh:\n%s\nvs\n%s", schemas[1], freshSchema)
	}
}

func TestPhase2Schema_OpenTargetIndexIsPartial_SQLite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	ins := func(id, kind, target, status string) error {
		_, err := store.db.ExecContext(ctx, store.db.Rebind(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, kind, target_task_id, created_at, updated_at) VALUES (?, ?, 'ws-1', ?, '{}', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`), id, c.ID, status, kind, target)
		return err
	}
	if err := ins("a", "create_task", "", "pending"); err != nil {
		t.Fatal(err)
	}
	if err := ins("b", "create_task", "", "pending"); err != nil {
		t.Fatalf("create_task rows must not collide: %v", err)
	}
	if err := ins("c", "move_task", "t1", "pending"); err != nil {
		t.Fatal(err)
	}
	if err := ins("d", "move_task", "t1", "failed"); err == nil {
		t.Fatal("second open proposal on the same target must be refused")
	}
	if err := ins("e", "move_task", "t1", "rejected"); err != nil {
		t.Fatalf("a closed row must not collide: %v", err)
	}
}

func TestPhase2Schema_UpgradeFromPhase1_Postgres(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	var schemas []string
	for _, withRevision := range []bool{false, true} {
		pg := testutil.OpenIsolatedPostgres(t, dsn)
		upgradeFromPhase1(t, pg, withRevision)
		assertUpgradedDefaults(t, pg)
		schemas = append(schemas, endSchema(t, pg))
	}
	if !strings.Contains(schemas[0], "policy_revision") {
		t.Fatal("phase-2 columns missing")
	}
}
