package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

// legacyDynamicRouteStatesDDL is the pre-#3362 dynamic_route_states shape:
// identical to the current fresh-install DDL minus the backfill marker
// column, so opening a database against it reproduces exactly what the
// migration must detect and repair.
const legacyDynamicRouteStatesDDL = `
	CREATE TABLE dynamic_route_states (
		session_id TEXT PRIMARY KEY,
		logical_profile_id TEXT NOT NULL,
		execution_profile_id TEXT NOT NULL DEFAULT '',
		route_generation BIGINT NOT NULL DEFAULT 0,
		profile_version BIGINT NOT NULL DEFAULT 0,
		state TEXT NOT NULL DEFAULT 'selecting',
		continuation_json TEXT NOT NULL DEFAULT '',
		policy_state_json TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);`

// openLegacyDynamicRouteDB builds a baseline database with the final schema,
// then rewinds dynamic_route_states to its pre-marker legacy shape so the
// backfill migration has something to detect.
func openLegacyDynamicRouteDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "legacy-dynamic-route.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed baseline schema: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE dynamic_route_states`); err != nil {
		t.Fatalf("drop final dynamic_route_states: %v", err)
	}
	if _, err := db.Exec(legacyDynamicRouteStatesDDL); err != nil {
		t.Fatalf("seed legacy dynamic_route_states: %v", err)
	}
	setStoredKandevVersion(t, db, "v0.93.0")
	return db
}

func seedLegacyDynamicRouteTaskAndSession(t *testing.T, db *sqlx.DB, taskID, sessionID, sessionState string, startedAt time.Time) {
	seedLegacyDynamicRouteTaskAndSessionAtGeneration(t, db, taskID, sessionID, sessionState, startedAt, 1)
}

func seedLegacyDynamicRouteTaskAndSessionAtGeneration(
	t *testing.T,
	db *sqlx.DB,
	taskID, sessionID, sessionState string,
	startedAt time.Time,
	generation int64,
) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'legacy dynamic route task', ?, ?)`), taskID, startedAt, startedAt); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, route_generation, route_state, started_at, updated_at)
		VALUES (?, ?, ?, ?, 'starting', ?, ?)`), sessionID, taskID, sessionState, generation, startedAt, startedAt); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func seedLegacyDynamicRouteState(t *testing.T, db *sqlx.DB, sessionID, state string, updatedAt time.Time) {
	seedLegacyDynamicRouteStateAtGeneration(t, db, sessionID, state, 1, updatedAt)
}

func seedLegacyDynamicRouteStateAtGeneration(
	t *testing.T,
	db *sqlx.DB,
	sessionID, state string,
	generation int64,
	updatedAt time.Time,
) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO dynamic_route_states (
			session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, state, updated_at
		) VALUES (?, 'dynamic-logical', 'candidate-1', ?, 1, ?, ?)`),
		sessionID, generation, state, updatedAt); err != nil {
		t.Fatalf("seed dynamic_route_states: %v", err)
	}
}

func setStoredKandevVersion(t *testing.T, db *sqlx.DB, version string) {
	t.Helper()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kandev_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("ensure kandev_meta: %v", err)
	}
	if version == "" {
		if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key = 'kandev_version'`); err != nil {
			t.Fatalf("delete kandev_version: %v", err)
		}
		return
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`), "kandev_version", version); err != nil {
		t.Fatalf("set kandev_version: %v", err)
	}
}

func dynamicRouteState(t *testing.T, db *sqlx.DB, sessionID string) string {
	t.Helper()
	var state string
	if err := db.QueryRow(db.Rebind(
		`SELECT state FROM dynamic_route_states WHERE session_id = ?`), sessionID).Scan(&state); err != nil {
		t.Fatalf("read dynamic_route_states.state: %v", err)
	}
	return state
}

func taskSessionRouteState(t *testing.T, db *sqlx.DB, sessionID string) string {
	t.Helper()
	var routeState string
	if err := db.QueryRow(db.Rebind(
		`SELECT route_state FROM task_sessions WHERE id = ?`), sessionID).Scan(&routeState); err != nil {
		t.Fatalf("read task_sessions.route_state: %v", err)
	}
	return routeState
}

// TestBackfillLegacyActiveDynamicRoutes_MigratesLegacyIdleRouteToActive is
// the regression test for the PR #3362 review follow-up (thread
// r3931855722): on a database created before the durable "active" route
// status existed, every currently-IDLE Office dynamic session's route is
// durably "starting" only because the marking mechanism did not exist yet.
// Without this backfill, the startup orphan sweep
// (isOrphanableDynamicSessionState) would misclassify every one of these
// healthy routes as orphaned on the first restart after upgrading.
func TestBackfillLegacyActiveDynamicRoutes_MigratesLegacyIdleRouteToActive(t *testing.T) {
	db := openLegacyDynamicRouteDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	seedLegacyDynamicRouteTaskAndSession(t, db, "task-legacy-idle", "session-legacy-idle", "IDLE", now)
	seedLegacyDynamicRouteState(t, db, "session-legacy-idle", "starting", now)

	// Control: a session still STARTING (a genuine in-flight launch at the
	// moment of the snapshot) must not be touched by the legacy backfill -
	// only isOrphanableDynamicSessionState's STARTING branch, evaluated live
	// at startup, may resolve it.
	seedLegacyDynamicRouteTaskAndSession(t, db, "task-legacy-starting", "session-legacy-starting", "STARTING", now)
	seedLegacyDynamicRouteState(t, db, "session-legacy-starting", "starting", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run legacy backfill migration: %v", err)
	}

	if got := dynamicRouteState(t, db, "session-legacy-idle"); got != "active" {
		t.Fatalf("dynamic_route_states.state for legacy IDLE route = %q, want active", got)
	}
	if got := taskSessionRouteState(t, db, "session-legacy-idle"); got != "active" {
		t.Fatalf("task_sessions.route_state for legacy IDLE route = %q, want active", got)
	}
	if got := dynamicRouteState(t, db, "session-legacy-starting"); got != "starting" {
		t.Fatalf("dynamic_route_states.state for STARTING session = %q, want unchanged starting", got)
	}
	if got := taskSessionRouteState(t, db, "session-legacy-starting"); got != "starting" {
		t.Fatalf("task_sessions.route_state for STARTING session = %q, want unchanged starting", got)
	}

	exists, err := dbutil.ColumnExists(db, "dynamic_route_states", dynamicRouteLegacyActiveBackfillColumn)
	if err != nil {
		t.Fatalf("probe marker column: %v", err)
	}
	if !exists {
		t.Fatal("marker column missing after migration")
	}

	// A route claimed after the marker column exists can still fail before
	// reaching "active" and land on the same starting+IDLE shape. Re-running
	// initSchema (as every boot does) must not backfill it: the marker gates
	// the whole migration, not just the value-based shape, so this row must
	// survive to be caught by the live startup orphan sweep instead.
	seedLegacyDynamicRouteTaskAndSession(t, db, "task-post-upgrade-orphan", "session-post-upgrade-orphan", "IDLE", now)
	seedLegacyDynamicRouteState(t, db, "session-post-upgrade-orphan", "starting", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay initSchema after marker column exists: %v", err)
	}

	if got := dynamicRouteState(t, db, "session-post-upgrade-orphan"); got != "starting" {
		t.Fatalf("post-upgrade orphan dynamic_route_states.state = %q, want unchanged starting (must survive to the runtime sweep)", got)
	}
	if got := taskSessionRouteState(t, db, "session-post-upgrade-orphan"); got != "starting" {
		t.Fatalf("post-upgrade orphan task_sessions.route_state = %q, want unchanged starting", got)
	}
}

// TestBackfillLegacyActiveDynamicRoutes_NoRouteRowLeavesProjectionUnchanged
// is the regression test for review round 1 finding F2 (negative case i): an
// IDLE session whose task_sessions.route_state projection reads 'starting'
// but has no dynamic_route_states row at all must not be touched. The
// projection UPDATE is scoped to rows the dynamic_route_states UPDATE just
// backfilled to 'active', so a session with no route row is never in scope.
func TestBackfillLegacyActiveDynamicRoutes_NoRouteRowLeavesProjectionUnchanged(t *testing.T) {
	db := openLegacyDynamicRouteDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	seedLegacyDynamicRouteTaskAndSession(t, db, "task-no-route-row", "session-no-route-row", "IDLE", now)
	// Deliberately no seedLegacyDynamicRouteState call: this session has no
	// dynamic_route_states row.

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run legacy backfill migration: %v", err)
	}

	if got := taskSessionRouteState(t, db, "session-no-route-row"); got != "starting" {
		t.Fatalf("task_sessions.route_state for session with no route row = %q, want unchanged starting", got)
	}
}

// TestBackfillLegacyActiveDynamicRoutes_ActionRequiredRouteLeavesProjectionUnchanged
// is the regression test for review round 1 finding F2 (negative case ii): an
// IDLE session whose durable dynamic_route_states row is 'action_required'
// (a genuinely broken route surfaced by routeDynamicAgentFailure) but whose
// task_sessions.route_state projection still reads 'starting' - because the
// projection write that follows the durable write in
// routeDynamicAgentFailure failed or never ran - must keep that projection
// untouched. Overwriting it with 'active' would hide a live Retry banner for
// a route that is actually broken.
func TestBackfillLegacyActiveDynamicRoutes_ActionRequiredRouteLeavesProjectionUnchanged(t *testing.T) {
	db := openLegacyDynamicRouteDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	seedLegacyDynamicRouteTaskAndSession(t, db, "task-action-required", "session-action-required", "IDLE", now)
	seedLegacyDynamicRouteState(t, db, "session-action-required", "action_required", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run legacy backfill migration: %v", err)
	}

	if got := dynamicRouteState(t, db, "session-action-required"); got != "action_required" {
		t.Fatalf("dynamic_route_states.state for action_required route = %q, want unchanged action_required", got)
	}
	if got := taskSessionRouteState(t, db, "session-action-required"); got != "starting" {
		t.Fatalf("task_sessions.route_state for action_required route = %q, want unchanged starting (must not hide the Retry banner)", got)
	}
}

// TestBackfillLegacyActiveDynamicRoutes_FreshInstallIsNoOp proves a brand new
// database (whose dynamic_route_states DDL already declares the marker
// column) never runs the backfill, so a genuinely orphaned starting+IDLE
// route created on a fresh install is never masked.
func TestBackfillLegacyActiveDynamicRoutes_FreshInstallIsNoOp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh-dynamic-route.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed baseline schema: %v", err)
	}

	exists, err := dbutil.ColumnExists(db, "dynamic_route_states", dynamicRouteLegacyActiveBackfillColumn)
	if err != nil {
		t.Fatalf("probe marker column: %v", err)
	}
	if !exists {
		t.Fatal("fresh install is missing the backfill marker column")
	}

	now := time.Now().UTC().Truncate(time.Second)
	seedLegacyDynamicRouteTaskAndSession(t, db, "task-fresh-orphan", "session-fresh-orphan", "IDLE", now)
	seedLegacyDynamicRouteState(t, db, "session-fresh-orphan", "starting", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay initSchema: %v", err)
	}

	if got := dynamicRouteState(t, db, "session-fresh-orphan"); got != "starting" {
		t.Fatalf("fresh-install orphan dynamic_route_states.state = %q, want unchanged starting", got)
	}
}

func TestBackfillLegacyActiveDynamicRoutes_SkipsUnprovenancedRoutes(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "unknown", version: ""},
		{name: "newer stable", version: "v0.94.0"},
		{name: "nightly provenance", version: "0.93.1-nightly.sha123456789abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openLegacyDynamicRouteDB(t)
			setStoredKandevVersion(t, db, tt.version)
			now := time.Now().UTC().Truncate(time.Second)
			seedLegacyDynamicRouteTaskAndSession(t, db, "task-unprovenanced", "session-unprovenanced", "IDLE", now)
			seedLegacyDynamicRouteState(t, db, "session-unprovenanced", "starting", now)

			if _, err := NewWithDB(db, db, nil); err != nil {
				t.Fatalf("run legacy backfill migration: %v", err)
			}
			if got := dynamicRouteState(t, db, "session-unprovenanced"); got != "starting" {
				t.Fatalf("dynamic_route_states.state = %q, want unchanged starting", got)
			}
			if got := taskSessionRouteState(t, db, "session-unprovenanced"); got != "starting" {
				t.Fatalf("task_sessions.route_state = %q, want unchanged starting", got)
			}
			exists, err := dbutil.ColumnExists(db, "dynamic_route_states", dynamicRouteLegacyActiveBackfillColumn)
			if err != nil {
				t.Fatalf("probe marker column: %v", err)
			}
			if !exists {
				t.Fatal("marker column missing after skipped migration")
			}
		})
	}
}

func TestBackfillLegacyActiveDynamicRoutes_SkipsGenerationMismatch(t *testing.T) {
	db := openLegacyDynamicRouteDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedLegacyDynamicRouteTaskAndSessionAtGeneration(t, db, "task-generation-mismatch", "session-generation-mismatch", "IDLE", now, 2)
	seedLegacyDynamicRouteStateAtGeneration(t, db, "session-generation-mismatch", "starting", 1, now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run legacy backfill migration: %v", err)
	}
	if got := dynamicRouteState(t, db, "session-generation-mismatch"); got != "starting" {
		t.Fatalf("dynamic_route_states.state = %q, want unchanged starting", got)
	}
	if got := taskSessionRouteState(t, db, "session-generation-mismatch"); got != "starting" {
		t.Fatalf("task_sessions.route_state = %q, want unchanged starting", got)
	}
}

func TestBackfillLegacyActiveDynamicRoutes_SkipsNonStartingProjection(t *testing.T) {
	db := openLegacyDynamicRouteDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedLegacyDynamicRouteTaskAndSession(t, db, "task-non-starting-projection", "session-non-starting-projection", "IDLE", now)
	seedLegacyDynamicRouteState(t, db, "session-non-starting-projection", "starting", now)
	if _, err := db.Exec(db.Rebind(
		`UPDATE task_sessions SET route_state = ? WHERE id = ?`,
	), "active", "session-non-starting-projection"); err != nil {
		t.Fatalf("set non-starting projection: %v", err)
	}

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("run legacy backfill migration: %v", err)
	}
	if got := dynamicRouteState(t, db, "session-non-starting-projection"); got != "starting" {
		t.Fatalf("dynamic_route_states.state = %q, want unchanged starting", got)
	}
	if got := taskSessionRouteState(t, db, "session-non-starting-projection"); got != "active" {
		t.Fatalf("task_sessions.route_state = %q, want unchanged active", got)
	}
}
