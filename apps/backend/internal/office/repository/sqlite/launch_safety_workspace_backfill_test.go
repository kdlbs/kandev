package sqlite_test

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestLaunchSafetyWorkspaceBackfill_RepairsLegacyActiveRuns pins Review
// round 16's RR16-F4: the runs.workspace_id ADD COLUMN migration defaults
// every existing row to ”, so every legacy run in flight when this branch
// first boots pools into the workspace-scoped concurrency ceiling and
// budget's empty-string bucket instead of its own workspace
// (AC-OFFICE-RUN-CAUSATION-001.19). Active (queued/claimed) rows whose
// agent profile is known must be repaired from agent_profiles.workspace_id;
// terminal rows and rows with an unresolvable agent profile are left alone.
func TestLaunchSafetyWorkspaceBackfill_RepairsLegacyActiveRuns(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(legacyRunsTableDDL); err != nil {
		t.Fatalf("seed legacy runs table: %v", err)
	}

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	seedAgentProfile(t, db, "agent-known", "ws-known")

	now := time.Now().UTC()
	seedRun := func(id, agentProfileID, status string) {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO runs (id, agent_profile_id, reason, status, requested_at)
			VALUES (?, ?, 'self', ?, ?)
		`), id, agentProfileID, status, now); err != nil {
			t.Fatalf("seed run %q: %v", id, err)
		}
	}
	seedRun("run-queued-known", "agent-known", "queued")
	seedRun("run-claimed-known", "agent-known", "claimed")
	seedRun("run-succeeded-known", "agent-known", "succeeded")
	seedRun("run-queued-unknown", "agent-missing", "queued")

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("boot against a pre-existing legacy-shaped runs table: %v", err)
	}

	assertRunWorkspaceID(t, db, "run-queued-known", "ws-known")
	assertRunWorkspaceID(t, db, "run-claimed-known", "ws-known")
	assertRunWorkspaceID(t, db, "run-succeeded-known", "")
	assertRunWorkspaceID(t, db, "run-queued-unknown", "")
}

func assertRunWorkspaceID(t *testing.T, db *sqlx.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.Get(&got, db.Rebind(`SELECT workspace_id FROM runs WHERE id = ?`), runID); err != nil {
		t.Fatalf("read workspace_id for %q: %v", runID, err)
	}
	if got != want {
		t.Errorf("workspace_id for %q = %q, want %q", runID, got, want)
	}
}
