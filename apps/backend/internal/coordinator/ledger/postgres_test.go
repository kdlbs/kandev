package ledger

import (
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

const externalTablesPostgresSQL = `
	CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, workflow_step_id TEXT, state TEXT, archived_at TIMESTAMPTZ, updated_at TIMESTAMPTZ);
	CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, complete_task_on_enter BOOLEAN NOT NULL DEFAULT FALSE);
	CREATE TABLE task_sessions (id TEXT PRIMARY KEY, state TEXT);
	CREATE TABLE task_session_turns (id TEXT PRIMARY KEY, started_at TIMESTAMPTZ);
	CREATE TABLE task_usage_events (id SERIAL PRIMARY KEY, session_id TEXT, turn_id TEXT, agent_type TEXT, model TEXT, tokens_in INTEGER NOT NULL DEFAULT 0, tokens_out INTEGER, tokens_total INTEGER NOT NULL DEFAULT 0, cost_subcents INTEGER NOT NULL DEFAULT 0, cost_source TEXT NOT NULL DEFAULT 'priced', created_at TIMESTAMPTZ);
`

// newPostgresFixture runs the ledger over a pool of several connections that
// all resolve one private schema, so concurrent callers really overlap.
func newPostgresFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	boot := testutil.OpenIsolatedPostgres(t, dsn)
	if _, err := boot.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = boot.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE") })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	raw, err := internaldb.OpenPostgres(u.String(), 8, 2)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	pool := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = pool.Close() })
	return newFixtureOn(t, pool, pool, externalTablesPostgresSQL)
}

func TestPostgres_StartRedeliveryKeepsOneRowAndOneSnapshot(t *testing.T) {
	f := newPostgresFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', ?), ('s-done', ?)`, false, true)
	f.task("t-open", "wf", "s-open", "TODO", f.at(0), false)
	f.task("t-done", "wf", "s-done", "COMPLETED", f.at(0), false)
	started := f.at(-time.Minute)
	f.start("st-1", started)
	f.start("st-1", started)
	row := f.oneTurn()
	if row.SnapshotHash == "" {
		t.Fatal("first row carries no snapshot hash")
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turn_snapshots`); n != 1 {
		t.Fatalf("snapshots = %d, want 1", n)
	}
}

func TestPostgres_SnapshotSelectsOnlyOpenWatchedTasks(t *testing.T) {
	f := newPostgresFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', ?), ('s-done', ?)`, false, true)
	f.task("t-b", "wf", "s-open", "TODO", f.at(time.Minute), false)
	f.task("t-a", "wf", "s-open", "TODO", f.at(time.Minute), false)
	f.task("t-archived", "wf", "s-open", "TODO", f.at(time.Hour), true)
	f.task("t-done", "wf", "s-done", "COMPLETED", f.at(time.Hour), false)
	f.watch = coordinator.WatchSet{All: true}
	_, body := f.snapshot(f.at(time.Hour))
	var ids []string
	for _, tk := range body.Tasks {
		ids = append(ids, tk.TaskID)
	}
	if strings.Join(ids, ",") != "t-a,t-b" || body.TasksTotal != 2 {
		t.Fatalf("tasks = %v total %d", ids, body.TasksTotal)
	}
}

func TestPostgres_ConcurrentCompletionFinishesTheRowOnce(t *testing.T) {
	f := newPostgresFixture(t)
	started, completed := f.at(-time.Minute), f.at(-time.Second)
	f.start("st-1", started)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.complete("st-1", started, completed)
		}()
	}
	wg.Wait()
	row := f.finishedTurn()
	if row.ID == "" || len(f.turns()) != 1 {
		t.Fatalf("rows = %d", len(f.turns()))
	}
}

func TestPostgres_PassFillsModelAndCorrectsNothingElse(t *testing.T) {
	f := newPostgresFixture(t)
	now := time.Now().UTC()
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", model, started_at, finished_at, outcome, verdict)
		VALUES ('empty', ?, 's', 'st-empty', 'message', '', ?, ?, 'completed', 'nothing_needed')`, f.coord.ID, now.Add(-time.Hour), now.Add(-time.Minute))
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, agent_type, model, created_at) VALUES ('s', 'st-empty', 'agent', ' Fresh-1 ', ?)`, now.Add(-time.Minute))
	before := FailureCount(StageSettle) + FailureCount(StageModel)
	f.l.RunPass(t.Context())
	if got := FailureCount(StageSettle) + FailureCount(StageModel); got != before {
		t.Fatalf("pass counted %d failures on postgres", got-before)
	}
	if row := f.oneTurn(); row.Model != "fresh-1" || row.Harness != "agent@v9" {
		t.Fatalf("model = %q harness %q", row.Model, row.Harness)
	}
}

func TestPostgres_RetentionDeletesOldTurnsCallsAndSnapshots(t *testing.T) {
	f := newPostgresFixture(t)
	day := 24 * time.Hour
	f.exec(`INSERT INTO coordinator_turn_snapshots (hash, body, created_at) VALUES ('orphan', '{}', ?), ('shared', '{}', ?)`, time.Now().UTC().Add(-500*day), time.Now().UTC().Add(-500*day))
	f.bulkTurns(batchSize+3, "old", 401*day, true, "")
	f.bulkTurns(1, "keep", 10*day, true, "shared")
	f.exec(`INSERT INTO coordinator_turn_calls (turn_id, action, allowed, recorded_at) VALUES ('old-0000', 'a', ?, ?), ('keep-0000', 'a', ?, ?)`, true, f.now, true, f.now)
	f.l.RunDaily(t.Context())
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns`); n != 1 {
		t.Fatalf("turns left = %d, want 1", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turn_calls`); n != 1 {
		t.Fatalf("calls left = %d, want 1", n)
	}
	var kept []string
	if err := f.db.Select(&kept, `SELECT hash FROM coordinator_turn_snapshots`); err != nil || strings.Join(kept, ",") != "shared" {
		t.Fatalf("snapshots kept = %v (%v)", kept, err)
	}
}
