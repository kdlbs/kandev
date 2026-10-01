package ledger

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	testConvTask = "conv-task"
	testSession  = "sess-1"
)

type fixture struct {
	t     *testing.T
	db    *sqlx.DB
	store *coordinator.Store
	coord *coordinator.Coordinator
	l     *Ledger
	now   time.Time

	clock      atomic.Pointer[time.Time]
	pending    bool
	pendingErr error
	watch      coordinator.WatchSet
}

const externalTablesSQL = `
	CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, workflow_step_id TEXT, state TEXT, archived_at DATETIME, updated_at DATETIME);
	CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, complete_task_on_enter INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE task_sessions (id TEXT PRIMARY KEY, state TEXT);
	CREATE TABLE task_session_turns (id TEXT PRIMARY KEY, started_at DATETIME);
	CREATE TABLE task_usage_events (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, turn_id TEXT, agent_type TEXT, model TEXT, tokens_in INTEGER NOT NULL DEFAULT 0, tokens_out INTEGER, tokens_total INTEGER NOT NULL DEFAULT 0, cost_subcents INTEGER NOT NULL DEFAULT 0, cost_source TEXT NOT NULL DEFAULT 'priced', created_at DATETIME);
`

func newFixture(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	wc, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = wc.Close() })
	rc, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	writer, reader := sqlx.NewDb(wc, "sqlite3"), sqlx.NewDb(rc, "sqlite3")
	store, err := coordinator.NewStore(writer, reader)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := writer.Exec(externalTablesSQL); err != nil {
		t.Fatalf("external tables: %v", err)
	}
	c := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Coord", AgentProfileID: "agent-1", ExecutorProfileID: "exec-1"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	f := &fixture{t: t, db: writer, store: store, coord: c, now: time.Now().UTC().Truncate(time.Millisecond), watch: coordinator.WatchSet{All: true}}
	f.l = New(Deps{
		DB: writer, RO: reader, BuildVersion: "v9",
		Now: func() time.Time {
			if at := f.clock.Load(); at != nil {
				return *at
			}
			return time.Now().UTC()
		},
		CoordinatorForTask: func(_ context.Context, taskID string) (string, bool, error) {
			return c.ID, taskID == testConvTask, nil
		},
		PromptHash: func(context.Context, string) string { return "prompt-hash" },
		WatchSet:   func(context.Context, string) (coordinator.WatchSet, error) { return f.watch, nil },
		ActionPending: func(context.Context, string) (bool, error) {
			return f.pending, f.pendingErr
		},
	})
	f.l.retryEvery = 5 * time.Millisecond
	t.Cleanup(f.l.Stop)
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(query), args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *fixture) at(offset time.Duration) time.Time { return f.now.Add(offset) }

func turnEventData(sessionTurnID string, started time.Time, completed *time.Time) *bus.Event {
	data := map[string]any{"id": sessionTurnID, "session_id": testSession, "task_id": testConvTask, "started_at": started}
	if completed != nil {
		data["completed_at"] = completed
	}
	return &bus.Event{Data: data}
}

func (f *fixture) start(sessionTurnID string, started time.Time) {
	f.l.OnTurnStarted(context.Background(), turnEventData(sessionTurnID, started, nil))
}

func (f *fixture) complete(sessionTurnID string, started, completed time.Time) {
	f.l.OnTurnCompleted(context.Background(), turnEventData(sessionTurnID, started, &completed))
}

type turnSnapshotRow struct {
	ID           string     `db:"id"`
	Trigger      string     `db:"trigger"`
	WakeKinds    string     `db:"wake_kinds"`
	Model        string     `db:"model"`
	Harness      string     `db:"harness"`
	PromptHash   string     `db:"prompt_hash"`
	SnapshotHash string     `db:"snapshot_hash"`
	Agent        string     `db:"agent_profile_id"`
	Outcome      *string    `db:"outcome"`
	Verdict      *string    `db:"verdict"`
	FinishedAt   *time.Time `db:"finished_at"`
	Truncated    bool       `db:"calls_truncated"`

	StartedAt      time.Time `db:"started_at"`
	WatchScope     string    `db:"watch_scope"`
	WatchIDs       string    `db:"watch_ids"`
	ConfigRevision int64     `db:"config_revision"`
	PolicyRevision int64     `db:"policy_revision"`
}

func (f *fixture) turns() []turnSnapshotRow {
	f.t.Helper()
	var rows []turnSnapshotRow
	err := f.db.Select(&rows, `SELECT id, "trigger", wake_kinds, model, harness, prompt_hash, snapshot_hash, agent_profile_id, outcome, verdict, finished_at, calls_truncated, started_at, watch_scope, watch_ids, config_revision, policy_revision FROM coordinator_turns ORDER BY started_at, id`)
	if err != nil {
		f.t.Fatalf("read turns: %v", err)
	}
	return rows
}

func (f *fixture) oneTurn() turnSnapshotRow {
	f.t.Helper()
	rows := f.turns()
	if len(rows) != 1 {
		f.t.Fatalf("ledger rows = %d, want 1", len(rows))
	}
	return rows[0]
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	for !cond() {
		select {
		case <-tick.C:
		case <-timeout:
			t.Fatal("condition not reached")
		}
	}
}

func (f *fixture) setClock(at time.Time) { f.clock.Store(&at) }

func (f *fixture) resetClock() { f.clock.Store(nil) }

var errTest = errors.New("test error")
