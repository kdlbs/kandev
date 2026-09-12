package sqlite_test

import (
	"context"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// TestPostgresListStuckParents_KeyedRunRetiresTimestampFallbackForParent is
// the PostgreSQL twin of TestListStuckParents_KeyedRunRetiresTimestampFallbackForParent.
// ListStuckParents' pre-upgrade compatibility clause (the
// "EXISTS wave-keyed run OR NOT EXISTS blocking pre-upgrade run" gate added
// alongside the wake_wave_string comparison) used two literal
// json_extract(w.payload, '$.task_id') expressions, which are a syntax
// error on Postgres (payload->>'task_id' is the Postgres form) — the same
// trap TestPostgresHasPriorTasklessFailedRun and TestPostgresHasInFlightRunForTask
// were written for. Running ListStuckParents itself against a real Postgres
// backend makes a dialect regression fail loudly instead of only in
// production, where ListStuckParents errors on every tick and the
// reconciler backstop silently stops finding any candidate at all.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckParents_KeyedRunRetiresTimestampFallbackForParent(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	const parentID = "pg-wave-parent-1"
	const agentID = parentID + "-agent"
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(10 * time.Minute)

	seedPostgresWaveTask(t, ctx, repo, parentID, "", oldTime)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
	); err != nil {
		t.Fatalf("mark parent as Office task: %v", err)
	}

	childID := parentID + "-child-0"
	seedPostgresWaveTask(t, ctx, repo, childID, parentID, newTime)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET state = 'COMPLETED' WHERE id = ?`, childID,
	); err != nil {
		t.Fatalf("complete child: %v", err)
	}

	// agent_profiles.agent_id carries an FK to agents(id) that Postgres
	// enforces (unlike SQLite, which leaves it unenforced by default) — an
	// empty agent_id needs a matching agents row, not NULL/omission.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at) VALUES ('', '', ?, ?)
	`, oldTime, oldTime); err != nil {
		t.Fatalf("seed empty-id agents row: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, status, created_at, updated_at)
		VALUES (?, '', ?, ?, 'idle', ?, ?)
	`, agentID, agentID, agentID, oldTime, oldTime); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`, "p-runner-"+parentID, parentID, agentID); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	// A pre-upgrade terminal run (no wave identity at all) requested after
	// the child completed: under the timestamp-only compatibility rule
	// alone this must still block the parent.
	seedPostgresWaveRun(t, ctx, repo, "pg-run-pre-upgrade", agentID, "task_children_completed",
		"finished", newTime.Add(time.Minute), parentID, "", "")

	before, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (pre-upgrade run blocking): %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("before = %#v, want no candidates — the pre-upgrade terminal run must still block", before)
	}

	// A wave-keyed run also exists for this parent, for an unrelated
	// (already-stale) wave — it must not itself block via the wave-string
	// comparison, only retire the timestamp fallback for this parent.
	seedPostgresWaveRun(t, ctx, repo, "pg-run-keyed", agentID, "task_children_completed",
		"finished", oldTime.Add(time.Minute), parentID, "stale-wave-key", parentID+"|some-other-child")

	after, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (wave-keyed run retires fallback): %v", err)
	}
	if len(after) != 1 || after[0].ParentTaskID != parentID {
		t.Fatalf("after = %#v, want exactly [%s] — a wave-keyed run must retire the timestamp "+
			"fallback for this parent even though a recent pre-upgrade terminal run exists", after, parentID)
	}
}

// seedPostgresWaveTask inserts a minimal tasks row against a real Postgres
// connection, with created_at/updated_at set explicitly (Postgres has no
// datetime('now', ...) SQLite function, so tests needing controlled
// ordering must bind an actual time.Time).
func seedPostgresWaveTask(
	t *testing.T, ctx context.Context, repo *sqlite.Repository,
	id, parentID string, updatedAt time.Time,
) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, parent_id, created_at, updated_at)
		VALUES (?, 'ws-1', 'Task', ?, ?, ?)
	`, id, parentID, updatedAt, updatedAt); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

// seedPostgresWaveRun inserts a runs row carrying the wave-identity
// columns (or, when waveKey/waveString are empty, a pre-upgrade run
// carrying neither), matching seedWakeRunWithWave's SQLite shape but bound
// against a real Postgres connection with parameterized timestamps.
func seedPostgresWaveRun(
	t *testing.T, ctx context.Context, repo *sqlite.Repository,
	runID, agentID, reason, status string, requestedAt time.Time,
	taskID, waveKey, waveString string,
) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, error_message,
			requested_at, wake_wave_key, wake_wave_string
		) VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)
	`, runID, agentID, reason, `{"task_id":"`+taskID+`"}`, status, requestedAt, waveKey, waveString); err != nil {
		t.Fatalf("seed run %s: %v", runID, err)
	}
}
