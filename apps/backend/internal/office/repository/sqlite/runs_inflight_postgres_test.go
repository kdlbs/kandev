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

// TestPostgresHasInFlightRunForTask is the PostgreSQL twin of
// TestHasInFlightRunForTask. HasInFlightRunForTask reads task_id out of the
// runs payload, and a literal json_extract(...) there is a syntax error on
// Postgres (payload->>'task_id' is the Postgres form) — the same trap
// TestPostgresHasPriorTasklessFailedRun was written for. Running the predicate
// against a real Postgres backend makes a dialect regression fail loudly
// instead of only in production, where its failure mode is a detector that
// errors on every tick and surfaces nothing.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresHasInFlightRunForTask(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// runs is created by the task repository's schema init, mirroring
	// production boot order (see failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	cases := []struct {
		status string
		want   bool
	}{
		{"queued", true},
		{"claimed", true},
		{"finished", false},
		{"failed", false},
		{"cancelled", false},
	}
	for _, tc := range cases {
		taskID := "pg-inflight-" + tc.status
		seedPostgresRun(t, ctx, repo, "pg-run-"+tc.status, "agent-a", tc.status, now,
			`{"task_id":"`+taskID+`"}`)

		got, err := repo.HasInFlightRunForTask(ctx, taskID)
		if err != nil {
			t.Fatalf("HasInFlightRunForTask(%s): %v", tc.status, err)
		}
		if got != tc.want {
			t.Errorf("HasInFlightRunForTask(status=%s) = %v, want %v", tc.status, got, tc.want)
		}
	}

	// Scoping: the in-flight rows above must not answer for a different task.
	got, err := repo.HasInFlightRunForTask(ctx, "pg-inflight-absent")
	if err != nil {
		t.Fatalf("HasInFlightRunForTask (absent task): %v", err)
	}
	if got {
		t.Error("got true, want false — no run carries this task_id")
	}
}

// TestPostgresListInflightRunsForWorkspace is the PostgreSQL twin of
// TestListInflightRunsForWorkspace_ReturnsQueuedAndClaimed. The halt sweep's
// run-inventory query extracts task_id out of the runs payload with
// dialect.JSONExtract, the same SQLite-json_extract-vs-Postgres-->>' branch
// TestPostgresHasInFlightRunForTask guards; this proves the query itself
// (agent scoping, taskless empty task_id, terminal exclusion) against a
// real Postgres backend.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListInflightRunsForWorkspace(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// runs is created by the task repository's schema init, and
	// agent_profiles by the agent settings store's, mirroring production
	// boot order (see base_test.go's newTestRepo).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	// agent_profiles.agent_id has a foreign key to the agents catalog table
	// (enforced on Postgres, unlike the unchecked SQLite unit test); both
	// profiles below reuse this one catalog row.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, "pg-agent-type", "test-agent", now, now); err != nil {
		t.Fatalf("seed agents catalog row: %v", err)
	}
	seedPostgresAgentProfile(t, ctx, repo, "pg-agent-1", "pg-agent-type", "pg-ws-1", now)
	seedPostgresAgentProfile(t, ctx, repo, "pg-agent-2", "pg-agent-type", "pg-ws-2", now)

	seedPostgresRun(t, ctx, repo, "pg-run-queued", "pg-agent-1", "queued", now, `{"task_id":"pg-task-1"}`)
	seedPostgresRun(t, ctx, repo, "pg-run-claimed", "pg-agent-1", "claimed", now, `{"task_id":"pg-task-2"}`)
	seedPostgresRun(t, ctx, repo, "pg-run-taskless", "pg-agent-1", "queued", now, `{}`)
	seedPostgresRun(t, ctx, repo, "pg-run-finished", "pg-agent-1", "finished", now, `{"task_id":"pg-task-3"}`)
	seedPostgresRun(t, ctx, repo, "pg-run-other-ws", "pg-agent-2", "queued", now, `{"task_id":"pg-task-4"}`)

	runs, err := repo.ListInflightRunsForWorkspace(ctx, "pg-ws-1")
	if err != nil {
		t.Fatalf("list inflight runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("inflight run count = %d, want 3: %+v", len(runs), runs)
	}
	byID := map[string]string{}
	for _, run := range runs {
		byID[run.RunID] = run.TaskID
	}
	if byID["pg-run-queued"] != "pg-task-1" || byID["pg-run-claimed"] != "pg-task-2" || byID["pg-run-taskless"] != "" {
		t.Fatalf("unexpected run task ids: %+v", byID)
	}
	if _, ok := byID["pg-run-finished"]; ok {
		t.Fatalf("terminal run leaked into result: %+v", byID)
	}
	if _, ok := byID["pg-run-other-ws"]; ok {
		t.Fatalf("cross-workspace run leaked into result: %+v", byID)
	}
}

// TestPostgresListLiveOfficeTaskIDsForWorkspace is the PostgreSQL twin of
// TestListLiveOfficeTaskIDsForWorkspace_ReturnsActiveSessions. The repeat
// pause sweep joins task_sessions to Office agent profiles, so this keeps the
// workspace boundary and active-state predicate covered on both SQL dialects.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListLiveOfficeTaskIDsForWorkspace(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, "pg-live-agent-type", "test-agent", now, now); err != nil {
		t.Fatalf("seed agents catalog row: %v", err)
	}
	seedPostgresAgentProfile(t, ctx, repo, "pg-live-agent-1", "pg-live-agent-type", "pg-live-ws-1", now)
	seedPostgresAgentProfile(t, ctx, repo, "pg-live-agent-2", "pg-live-agent-type", "pg-live-ws-2", now)

	for _, session := range []struct {
		id, taskID, profileID, state string
	}{
		{"pg-session-created", "pg-task-created", "pg-live-agent-1", "CREATED"},
		{"pg-session-running", "pg-task-running", "pg-live-agent-1", "RUNNING"},
		{"pg-session-waiting", "pg-task-waiting", "pg-live-agent-1", "WAITING_FOR_INPUT"},
		{"pg-session-completed", "pg-task-completed", "pg-live-agent-1", "COMPLETED"},
		{"pg-session-other-ws", "pg-task-other", "pg-live-agent-2", "RUNNING"},
	} {
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, session.id, session.taskID, session.profileID, session.state, now, now); err != nil {
			t.Fatalf("seed session %s: %v", session.id, err)
		}
	}

	ids, err := repo.ListLiveOfficeTaskIDsForWorkspace(ctx, "pg-live-ws-1")
	if err != nil {
		t.Fatalf("list live Office task ids: %v", err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got["pg-task-created"] || !got["pg-task-running"] || !got["pg-task-waiting"] ||
		got["pg-task-completed"] || got["pg-task-other"] {
		t.Fatalf("live task ids = %v, want active tasks from pg-live-ws-1 only", got)
	}
}

// seedPostgresAgentProfile inserts a minimal agent_profiles row against a
// real Postgres connection, scoped to workspaceID so
// ListInflightRunsForWorkspace's agent_profiles join can resolve it.
// agentTypeID must already exist in the agents catalog table (Postgres
// enforces the foreign key that the SQLite unit tests leave unchecked).
func seedPostgresAgentProfile(
	t *testing.T, ctx context.Context, repo *sqlite.Repository,
	id, agentTypeID, workspaceID string, now time.Time,
) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, 'Agent', 'Agent', ?, 'engineer', ?, ?)
	`, id, agentTypeID, workspaceID, now, now); err != nil {
		t.Fatalf("seed agent profile %s: %v", id, err)
	}
}
