// Package sqlite_test covers the runs queue repository.
//
// The runs / run_events tables are created by the office repository's
// initSchema (see internal/office/repository/sqlite/base.go), so the
// harness builds an office repository and takes its embedded runs
// handle rather than hand-rolling the DDL — that keeps these tests
// honest about the schema production actually runs against.
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// newTestRepo returns a runs repository backed by a temp-dir SQLite
// database with the production writer/reader handle split.
func newTestRepo(t *testing.T) *runssqlite.Repository {
	t.Helper()
	repo, _, _ := newTestRepoWithHandles(t)
	return repo
}

// newTestRepoWithDB additionally hands back the writer handle so tests
// that need to seed foreign tables (agent_profiles, office_cost_events)
// can insert rows the runs repository has no typed method for.
func newTestRepoWithDB(t *testing.T) (*runssqlite.Repository, *sqlx.DB) {
	t.Helper()
	repo, writer, _ := newTestRepoWithHandles(t)
	return repo, writer
}

// newTestRepoWithHandles builds the repository over two genuinely
// distinct pools — the single-connection writer and the multi-
// connection reader — exactly as internal/persistence wires them.
// Keeping them distinct is what makes the writer/reader split
// testable: closing one pool disables every method that uses it.
func newTestRepoWithHandles(t *testing.T) (*runssqlite.Repository, *sqlx.DB, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runs.db")

	writerRaw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	readerRaw, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	// agent_profiles lives in the settings store; ListRuns joins it.
	if _, _, err := settingsstore.Provide(writer, reader, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("office repo init: %v", err)
	}
	return officeRepo.RunsRepository(), writer, reader
}

// seedAgentProfile inserts the minimal agent_profiles row ListRuns
// needs to resolve a run's workspace. agent_profiles.agent_id is a
// foreign key into agents (and the harness DSN enables FK enforcement),
// so each profile gets its own parent agent row in the same workspace —
// sharing one agent row across profiles would leave it pointing at
// whichever workspace was seeded first.
func seedAgentProfile(t *testing.T, writer *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := writer.Exec(`INSERT INTO agents
		(id, name, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, id, "agent-"+id, workspaceID, now, now); err != nil {
		t.Fatalf("seed agent %q: %v", id, err)
	}
	_, err := writer.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, workspace_id, role,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		id, id, "name", "display", workspaceID, now, now)
	if err != nil {
		t.Fatalf("seed agent_profile %q: %v", id, err)
	}
}

// seedCostEvent inserts one office_cost_events row for GetRunWithCosts.
func seedCostEvent(
	t *testing.T, writer *sqlx.DB, id, taskID string,
	tokensIn, tokensCachedIn, tokensOut, costSubcents int64,
) {
	t.Helper()
	now := time.Now().UTC()
	_, err := writer.Exec(`INSERT INTO office_cost_events
		(id, session_id, task_id, agent_profile_id, project_id, model, provider,
		 tokens_in, tokens_cached_in, tokens_out, cost_subcents, estimated,
		 occurred_at, created_at)
		VALUES (?, '', ?, 'a1', '', 'model', 'provider', ?, ?, ?, ?, 0, ?, ?)`,
		id, taskID, tokensIn, tokensCachedIn, tokensOut, costSubcents, now, now)
	if err != nil {
		t.Fatalf("seed cost event %q: %v", id, err)
	}
}

// mustCreateRun creates a run and fails the test on error.
func mustCreateRun(t *testing.T, repo *runssqlite.Repository, run *models.Run) *models.Run {
	t.Helper()
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

// mustGetRun reads a run back and fails the test when it is missing.
func mustGetRun(t *testing.T, repo *runssqlite.Repository, id string) *models.Run {
	t.Helper()
	run, err := repo.GetRunByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get run %q: %v", id, err)
	}
	return run
}

// setRequestedAt backdates a run so ordering assertions do not depend
// on wall-clock resolution between two CreateRun calls.
func setRequestedAt(t *testing.T, repo *runssqlite.Repository, id string, ts time.Time) {
	t.Helper()
	if err := repo.SetRunRequestedAtForTest(context.Background(), id, ts); err != nil {
		t.Fatalf("set requested_at for %q: %v", id, err)
	}
}

// setStatus forces status + timing fields on a seeded run.
func setStatus(
	t *testing.T, repo *runssqlite.Repository, id, status string,
	claimedAt, finishedAt *time.Time,
) {
	t.Helper()
	if err := repo.SetRunStatusForTest(context.Background(), id, status, claimedAt, finishedAt); err != nil {
		t.Fatalf("set status for %q: %v", id, err)
	}
}

func strPtr(s string) *string { return &s }

func timePtr(ts time.Time) *time.Time { return &ts }

// TestWriterAndReaderExposeDistinctHandles pins the accessors used by
// callers that join runs against their own tables: they must hand back
// the two handles NewWithDB was given, not the same one twice.
func TestWriterAndReaderExposeDistinctHandles(t *testing.T) {
	repo, writer, reader := newTestRepoWithHandles(t)

	if repo.Writer() != writer {
		t.Errorf("Writer() returned a different handle than the one passed to NewWithDB")
	}
	if repo.Reader() != reader {
		t.Errorf("Reader() returned a different handle than the one passed to NewWithDB")
	}
	if repo.Writer() == repo.Reader() {
		t.Fatalf("Writer() and Reader() returned the same handle")
	}

	if _, err := repo.Writer().Exec(
		`INSERT INTO runs (id, agent_profile_id, reason, requested_at) VALUES ('w1','a1','r',?)`,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("writer handle should accept writes: %v", err)
	}
	var seen int
	if err := repo.Reader().Get(&seen, `SELECT COUNT(*) FROM runs WHERE id = 'w1'`); err != nil {
		t.Fatalf("read through the reader handle: %v", err)
	}
	if seen != 1 {
		t.Errorf("reader saw %d rows for the writer's insert, want 1", seen)
	}
}

// TestReadPathsUseTheReadOnlyHandle is the behavioural half of the
// writer/reader split: every read method must go through r.ro, and
// every write through r.db. A write routed to the read-only reader
// would fail outright, and a read that still works after the writer
// pool is closed proves it did not use r.db.
func TestReadPathsUseTheReadOnlyHandle(t *testing.T) {
	repo, writer, _ := newTestRepoWithHandles(t)
	ctx := context.Background()

	run := mustCreateRun(t, repo, &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	})

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	got, err := repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunByID after closing the writer: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("GetRunByID returned %q, want %q", got.ID, run.ID)
	}
	if _, err := repo.ListPendingRunsForTask(ctx, "t1"); err != nil {
		t.Errorf("ListPendingRunsForTask after closing the writer: %v", err)
	}
	if _, err := repo.ListRunEvents(ctx, run.ID, -1, 0); err != nil {
		t.Errorf("ListRunEvents after closing the writer: %v", err)
	}

	// Writes must not fall back to the reader.
	if err := repo.FinishRun(ctx, run.ID, "finished", nil); err == nil {
		t.Errorf("FinishRun succeeded with a closed writer; the write path is not using r.db")
	}
}
