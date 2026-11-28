package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// legacyNativeRouteOwnerDDL is the upstream dynamic route storage shape before
// native route owners: both tables are keyed only by a task-session FK and the
// attempt journal has no selection ordinal yet.
const legacyNativeRouteOwnerDDL = `
	CREATE TABLE dynamic_route_states (
		session_id TEXT PRIMARY KEY,
		logical_profile_id TEXT NOT NULL,
		execution_profile_id TEXT NOT NULL DEFAULT '',
		route_generation BIGINT NOT NULL DEFAULT 0,
		profile_version BIGINT NOT NULL DEFAULT 0,
		state TEXT NOT NULL DEFAULT 'selecting',
		continuation_json TEXT NOT NULL DEFAULT '',
		policy_state_json TEXT NOT NULL DEFAULT '',
		legacy_active_backfill_applied INTEGER NOT NULL DEFAULT 1,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE TABLE dynamic_route_attempts (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		logical_profile_id TEXT NOT NULL,
		execution_profile_id TEXT NOT NULL DEFAULT '',
		route_generation BIGINT NOT NULL,
		profile_version BIGINT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		UNIQUE (session_id, route_generation),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX idx_dynamic_route_attempts_session
		ON dynamic_route_attempts(session_id, route_generation);`

// nativeRouteOwnerSQLiteCutoverSteps lists every SQLite failpoint of the
// native route owner cutover, in execution order.
var nativeRouteOwnerSQLiteCutoverSteps = []string{
	"create_state_shadow", "copy_states", "create_attempt_shadow", "copy_attempts",
	"drop_legacy_attempts", "drop_legacy_states", "rename_states", "rename_attempts",
	"owner_indexes",
}

var nativeRouteOwnerSeedTime = time.Date(2026, 11, 7, 9, 30, 15, 123456000, time.UTC)

func openLegacyNativeRouteOwnerSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "legacy-native-route-owner.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed baseline schema: %v", err)
	}
	rewindNativeRouteOwnersToLegacy(t, db)
	return db
}

// rewindNativeRouteOwnersToLegacy replaces the final-shaped route tables with
// their legacy shape and seeds task-session owned rows with distinct values
// and sub-second timestamps.
func rewindNativeRouteOwnersToLegacy(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, query := range []string{`DROP TABLE dynamic_route_attempts`, `DROP TABLE dynamic_route_states`, legacyNativeRouteOwnerDDL} {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("rewind route tables to legacy shape: %v", err)
		}
	}
	seedLegacyTask(t, db, legacySeed{taskID: "task-route-owner-a", sessionID: "session-route-owner-a"}, nativeRouteOwnerSeedTime)
	seedLegacyTask(t, db, legacySeed{taskID: "task-route-owner-b", sessionID: "session-route-owner-b"}, nativeRouteOwnerSeedTime)
	states := []struct {
		sessionID, state, continuation, policy string
		generation, version, backfill          int64
		updatedAt                              time.Time
	}{
		{"session-route-owner-a", "active", `{"turn":"a"}`, `{"policy":"a"}`, 3, 2, 0, nativeRouteOwnerSeedTime},
		{"session-route-owner-b", "selecting", "", "", 1, 1, 1, nativeRouteOwnerSeedTime.Add(90 * time.Minute)},
	}
	for _, s := range states {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO dynamic_route_states (
				session_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
				state, continuation_json, policy_state_json, legacy_active_backfill_applied, updated_at
			) VALUES (?, 'logical-owner', 'execution-owner', ?, ?, ?, ?, ?, ?, ?)`),
			s.sessionID, s.generation, s.version, s.state, s.continuation, s.policy, s.backfill, s.updatedAt); err != nil {
			t.Fatalf("seed legacy route state %s: %v", s.sessionID, err)
		}
	}
	attempts := []struct {
		id, sessionID, execution, reason string
		generation                       int64
		createdAt                        time.Time
	}{
		{"attempt-route-owner-a1", "session-route-owner-a", "execution-a1", "initial", 1, nativeRouteOwnerSeedTime.Add(time.Second)},
		{"attempt-route-owner-a2", "session-route-owner-a", "execution-a2", "fallback", 2, nativeRouteOwnerSeedTime.Add(2 * time.Second)},
		{"attempt-route-owner-b1", "session-route-owner-b", "execution-b1", "", 1, nativeRouteOwnerSeedTime.Add(3 * time.Second)},
	}
	for _, a := range attempts {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO dynamic_route_attempts (
				id, session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, reason, created_at
			) VALUES (?, ?, 'logical-owner', ?, ?, 1, ?, ?)`),
			a.id, a.sessionID, a.execution, a.generation, a.reason, a.createdAt); err != nil {
			t.Fatalf("seed legacy route attempt %s: %v", a.id, err)
		}
	}
}

// legacyNativeRouteOwnerRows dumps every legacy column of both route tables.
// Timestamps are compared as stored text so precision loss is visible.
func legacyNativeRouteOwnerRows(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	return dumpNativeRouteOwnerQuery(t, db, `
		SELECT session_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			state, continuation_json, policy_state_json, legacy_active_backfill_applied, CAST(updated_at AS TEXT)
		FROM dynamic_route_states ORDER BY session_id`,
		`SELECT id, session_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			reason, CAST(created_at AS TEXT)
		FROM dynamic_route_attempts ORDER BY id`)
}

// preCutoverNativeRouteOwnerRows dumps every column the cutover receives: the
// legacy columns plus the selection ordinal added by the preceding boot step.
func preCutoverNativeRouteOwnerRows(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	return dumpNativeRouteOwnerQuery(t, db, `
		SELECT session_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			state, continuation_json, policy_state_json, legacy_active_backfill_applied, CAST(updated_at AS TEXT)
		FROM dynamic_route_states ORDER BY session_id`,
		`SELECT id, session_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			reason, CAST(created_at AS TEXT), selection_ordinal
		FROM dynamic_route_attempts ORDER BY id`)
}

// finalNativeRouteOwnerRows dumps every final column, including the native
// owner key and selection ordinal.
func finalNativeRouteOwnerRows(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	return dumpNativeRouteOwnerQuery(t, db, `
		SELECT session_id, route_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			state, continuation_json, policy_state_json, legacy_active_backfill_applied, CAST(updated_at AS TEXT)
		FROM dynamic_route_states ORDER BY COALESCE(route_id, session_id)`,
		`SELECT id, session_id, route_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			reason, CAST(created_at AS TEXT), selection_ordinal
		FROM dynamic_route_attempts ORDER BY id`)
}

// nativeRouteOwnerSchema dumps the stored schema of both route tables and any
// leftover shadow table, so rollback can be compared byte for byte.
func nativeRouteOwnerSchema(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	const tables = `'dynamic_route_states', 'dynamic_route_attempts',
		'dynamic_route_states_native_shadow', 'dynamic_route_attempts_native_shadow'`
	if !dialect.IsPostgres(db.DriverName()) {
		return dumpNativeRouteOwnerQuery(t, db, `
			SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master
			WHERE tbl_name IN (`+tables+`) ORDER BY type, name`)
	}
	return dumpNativeRouteOwnerQuery(t, db, `
		SELECT table_name, column_name, data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name IN (`+tables+`)
		ORDER BY table_name, ordinal_position`,
		`SELECT c.relname, con.conname, pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relname IN (`+tables+`)
		ORDER BY c.relname, con.conname`,
		`SELECT tablename, indexname, indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename IN (`+tables+`)
		ORDER BY tablename, indexname`)
}

func dumpNativeRouteOwnerQuery(t *testing.T, db *sqlx.DB, queries ...string) []string {
	t.Helper()
	var out []string
	for _, query := range queries {
		rows, err := db.Queryx(query)
		if err != nil {
			t.Fatalf("dump route owner rows: %v", err)
		}
		for rows.Next() {
			values, err := rows.SliceScan()
			if err != nil {
				_ = rows.Close()
				t.Fatalf("scan route owner row: %v", err)
			}
			fields := make([]string, len(values))
			for i, value := range values {
				if raw, ok := value.([]byte); ok {
					value = string(raw)
				}
				fields[i] = fmt.Sprintf("%v", value)
			}
			out = append(out, strings.Join(fields, "|"))
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close route owner rows: %v", err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate route owner rows: %v", err)
		}
	}
	return out
}

func assertNativeRouteOwnerDump(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s changed:\n got: %q\nwant: %q", label, got, want)
	}
}

func assertNativeRouteOwnerFinalShape(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, table := range []string{"dynamic_route_states", "dynamic_route_attempts"} {
		for _, column := range []string{"route_id", "session_id"} {
			exists, err := dbutil.ColumnExists(db, table, column)
			if err != nil {
				t.Fatalf("probe %s.%s: %v", table, column, err)
			}
			if !exists {
				t.Fatalf("%s.%s missing after cutover", table, column)
			}
		}
	}
	if exists, err := dbutil.ColumnExists(db, "dynamic_route_attempts", "selection_ordinal"); err != nil || !exists {
		t.Fatalf("dynamic_route_attempts.selection_ordinal exists=%v err=%v", exists, err)
	}
	// Exactly one owner is required, and a session row and a native row share
	// the COALESCE owner uniqueness.
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_states (session_id, route_id, logical_profile_id, updated_at)
		VALUES (NULL, NULL, 'logical-owner', ?)`), nativeRouteOwnerSeedTime); err == nil {
		t.Fatal("route state without any owner was accepted")
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_states (session_id, route_id, logical_profile_id, updated_at)
		VALUES (?, ?, 'logical-owner', ?)`), "session-route-owner-a", "utility:both", nativeRouteOwnerSeedTime); err == nil {
		t.Fatal("route state with two owners was accepted")
	}
}

// insertNativeRouteOwnerRows writes one native (non task-session) state and
// attempt after the cutover so replay must preserve the new owner column too.
func insertNativeRouteOwnerRows(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_states (
			session_id, route_id, logical_profile_id, execution_profile_id, route_generation, profile_version,
			state, continuation_json, policy_state_json, legacy_active_backfill_applied, updated_at
		) VALUES (NULL, 'utility:call-owner', 'logical-owner', 'execution-native', 1, 4, 'active', '', '{"native":true}', 1, ?)`),
		nativeRouteOwnerSeedTime.Add(time.Hour)); err != nil {
		t.Fatalf("insert native route state: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_attempts (
			id, session_id, route_id, logical_profile_id, execution_profile_id, route_generation,
			profile_version, reason, created_at, selection_ordinal
		) VALUES ('attempt-route-owner-native', NULL, 'utility:call-owner', 'logical-owner', 'execution-native', 1, 4, 'initial', ?, 9)`),
		nativeRouteOwnerSeedTime.Add(time.Hour)); err != nil {
		t.Fatalf("insert native route attempt: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_attempts (
			id, session_id, route_id, logical_profile_id, execution_profile_id, route_generation,
			profile_version, reason, created_at, selection_ordinal
		) VALUES ('attempt-route-owner-native-dup', NULL, 'utility:call-owner', 'logical-owner', 'execution-native', 1, 4, '', ?, 10)`),
		nativeRouteOwnerSeedTime.Add(time.Hour)); err == nil {
		t.Fatal("duplicate native owner generation was accepted")
	}
}

// runNativeRouteOwnerUpgradeAndReplay boots the legacy database twice and
// proves the cutover preserves every legacy value and timestamp, and that a
// replay preserves both task-session and native-owner rows.
func runNativeRouteOwnerUpgradeAndReplay(t *testing.T, db *sqlx.DB) {
	t.Helper()
	before := legacyNativeRouteOwnerRows(t, db)
	if len(before) != 5 {
		t.Fatalf("legacy seed rows = %d, want 5", len(before))
	}
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run native route owner cutover: %v", err)
	}
	assertNativeRouteOwnerFinalShape(t, db)
	assertNativeRouteOwnerDump(t, "legacy route rows after cutover", legacyNativeRouteOwnerRows(t, db), before)

	insertNativeRouteOwnerRows(t, db)
	final := finalNativeRouteOwnerRows(t, db)
	schema := nativeRouteOwnerSchema(t, db)
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay native route owner cutover: %v", err)
	}
	assertNativeRouteOwnerDump(t, "route rows after replay", finalNativeRouteOwnerRows(t, db), final)
	assertNativeRouteOwnerDump(t, "route schema after replay", nativeRouteOwnerSchema(t, db), schema)
}

// runNativeRouteOwnerRollbackAt injects a failure after one cutover step and
// proves the legacy schema and rows are restored byte for byte, then that a
// clean retry completes the cutover with the same values.
func runNativeRouteOwnerRollbackAt(t *testing.T, db *sqlx.DB, step string) {
	t.Helper()
	repo := &Repository{db: db, ro: db, migrate: dbutil.NewMigrateLogger(db, nil)}
	// The ordinal column is added by an earlier, separately committed boot
	// step; apply it first so the snapshot is exactly the cutover's input.
	if err := repo.ensureRouteSelectionOrdinal(context.Background()); err != nil {
		t.Fatalf("prepare selection ordinal: %v", err)
	}
	if _, err := db.Exec(`UPDATE dynamic_route_attempts SET selection_ordinal = 7 WHERE id = 'attempt-route-owner-a2'`); err != nil {
		t.Fatalf("seed selection ordinal: %v", err)
	}
	schema := nativeRouteOwnerSchema(t, db)
	rows := preCutoverNativeRouteOwnerRows(t, db)

	repo.failNativeRouteOwnerCutoverAfter = step
	err := repo.initSchema()
	if err == nil || !strings.Contains(err.Error(), "injected failure after "+step) {
		t.Fatalf("initSchema with failpoint %s = %v, want injected failure", step, err)
	}
	assertNativeRouteOwnerDump(t, "route schema after rollback at "+step, nativeRouteOwnerSchema(t, db), schema)
	assertNativeRouteOwnerDump(t, "route rows after rollback at "+step, preCutoverNativeRouteOwnerRows(t, db), rows)

	repo.failNativeRouteOwnerCutoverAfter = ""
	if err := repo.initSchema(); err != nil {
		t.Fatalf("retry native route owner cutover after rollback at %s: %v", step, err)
	}
	assertNativeRouteOwnerFinalShape(t, db)
	assertNativeRouteOwnerDump(t, "route rows after retry at "+step, preCutoverNativeRouteOwnerRows(t, db), rows)
}

func TestNativeRouteOwnerMigration_FreshSchemaIsFinalAndReplays(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "fresh-native-route-owner.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh schema: %v", err)
	}
	seedLegacyTask(t, db, legacySeed{taskID: "task-route-owner-a", sessionID: "session-route-owner-a"}, nativeRouteOwnerSeedTime)
	assertNativeRouteOwnerFinalShape(t, db)
	schema := nativeRouteOwnerSchema(t, db)
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay fresh schema: %v", err)
	}
	assertNativeRouteOwnerDump(t, "fresh route schema after replay", nativeRouteOwnerSchema(t, db), schema)
}

func TestNativeRouteOwnerMigration_PreservesLegacyValuesAndReplays(t *testing.T) {
	runNativeRouteOwnerUpgradeAndReplay(t, openLegacyNativeRouteOwnerSQLite(t))
}

func TestNativeRouteOwnerMigration_RollbackAtEveryFailpoint(t *testing.T) {
	for _, step := range nativeRouteOwnerSQLiteCutoverSteps {
		t.Run(step, func(t *testing.T) {
			runNativeRouteOwnerRollbackAt(t, openLegacyNativeRouteOwnerSQLite(t), step)
		})
	}
}
