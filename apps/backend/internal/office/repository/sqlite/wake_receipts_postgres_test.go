package sqlite_test

import (
	"context"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListStuckParents_ReturnsResolvableCandidate is the PostgreSQL
// twin of the SQLite ListStuckParents coverage in wake_receipts_test.go.
// ListStuckParents' query mixed four SQLite-only constructs — GROUP_CONCAT,
// json_extract, a bare `IS NOT <expr>`, and RunnerProjection's `wsp.rowid` —
// so it fails outright on Postgres, the sole candidate source for
// ParentWakeReconciler's level-triggered backstop sweep. Running a real,
// fully resolvable candidate through the query (not just one construct in
// isolation) proves the whole statement plans and executes on Postgres and
// returns the right row, including a child-set key byte-identical to the
// one GetChildSetKey computes for the same child set.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckParents_ReturnsResolvableCandidate(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// agent_profiles (and its `agents` FK target) are owned by the settings
	// store schema; ListStuckParents INNER JOINs agent_profiles, so it must
	// exist first, mirroring production boot order (see
	// participant_claim_postgres_test.go).
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	const parentID, agentID = "pg-stuck-parent", "pg-stuck-agent"
	childID := parentID + "-child-0"

	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, project_id, created_at, updated_at)
		VALUES (?, 'ws-1', 'Parent', 'office-project', ?, ?)
	`), parentID, now, now); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, parent_id, state, created_at, updated_at)
		VALUES (?, 'ws-1', 'Child', ?, 'COMPLETED', ?, ?)
	`), childID, parentID, now, now); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), agentID, agentID, now, now); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'ws-1', '', 'idle', ?, ?)
	`), agentID, agentID, agentID, agentID, now, now); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`), "pg-runner-"+parentID, parentID, agentID); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != parentID {
		t.Fatalf("candidates = %#v, want exactly [%s]", candidates, parentID)
	}
	if candidates[0].AssigneeAgentProfileID != agentID {
		t.Fatalf("assignee = %q, want %q", candidates[0].AssigneeAgentProfileID, agentID)
	}

	wantKey := childID + ":COMPLETED"
	if candidates[0].ChildSetKey != wantKey {
		t.Fatalf("child set key = %q, want %q", candidates[0].ChildSetKey, wantKey)
	}
	gotKey, err := repo.GetChildSetKey(ctx, parentID)
	if err != nil {
		t.Fatalf("GetChildSetKey: %v", err)
	}
	if gotKey != wantKey {
		t.Fatalf("GetChildSetKey = %q, want %q (must match ListStuckParents' SQL-computed key)", gotKey, wantKey)
	}
}

// TestPostgresListStuckParents_ExcludesParentWithCoveredRun is the
// PostgreSQL twin of TestListStuckParents_ExcludesParentWithCoveredRun,
// exercising the NOT EXISTS-against-runs arm (json_extract) and the wave
// EXISTS arm together — both of which read w.payload via json_extract, one
// of the four SQLite-only constructs this fix replaces.
func TestPostgresListStuckParents_ExcludesParentWithCoveredRun(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	const parentID, agentID = "pg-covered-parent", "pg-covered-agent"
	childID := parentID + "-child-0"

	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, project_id, created_at, updated_at)
		VALUES (?, 'ws-1', 'Parent', 'office-project', ?, ?)
	`), parentID, now, now); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, parent_id, state, created_at, updated_at)
		VALUES (?, 'ws-1', 'Child', ?, 'COMPLETED', ?, ?)
	`), childID, parentID, now, now); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), agentID, agentID, now, now); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'ws-1', '', 'idle', ?, ?)
	`), agentID, agentID, agentID, agentID, now, now); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`), "pg-runner-"+parentID, parentID, agentID); err != nil {
		t.Fatalf("seed runner: %v", err)
	}
	// A queued task_children_completed run already covers this parent, keyed
	// by task_id inside the JSON payload — the json_extract/->>'task_id'
	// divergence this fix closes.
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, ?, 'task_children_completed', ?, 'queued', ?)
	`), "pg-run-"+parentID, agentID, `{"task_id":"`+parentID+`"}`, now); err != nil {
		t.Fatalf("seed covering run: %v", err)
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected the already-covered parent to be excluded, got %#v", candidates)
	}
}
