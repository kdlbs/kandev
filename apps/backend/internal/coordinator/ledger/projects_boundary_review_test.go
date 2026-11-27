package ledger

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/watch"
)

func testLedgerProjectsBoundary(t *testing.T, f *fixture) {
	t.Helper()
	f.exec(`CREATE TABLE repositories (id TEXT PRIMARY KEY, workspace_id TEXT, deleted_at TEXT);
		CREATE TABLE repository_sets (id TEXT PRIMARY KEY, workspace_id TEXT);
		CREATE TABLE repository_set_items (repository_set_id TEXT, repository_id TEXT);
		CREATE TABLE task_repositories (task_id TEXT, repository_id TEXT);
		INSERT INTO repositories VALUES ('repo', 'ws-1', NULL), ('foreign', 'elsewhere', NULL), ('deleted', 'ws-1', 'deleted');
		INSERT INTO repository_sets VALUES ('set', 'ws-1');
		INSERT INTO repository_set_items VALUES ('set', 'repo'), ('set', 'foreign'), ('set', 'deleted');
		INSERT INTO task_repositories VALUES ('in', 'repo'), ('out', 'foreign'), ('deleted', 'deleted'), ('dangling', 'missing')`)
	for _, id := range []string{"in", "out", "none", "deleted", "dangling"} {
		f.task(id, "wf", "step", "TODO", f.now, false)
	}
	f.watch = coordinator.WatchSet{All: true, Projects: watch.Scope{Selected: true, SetIDs: []string{"set"}}}
	_, body := f.snapshot(f.at(time.Hour))
	if len(body.Tasks) != 1 || body.Tasks[0].TaskID != "in" || body.TasksTotal != 1 {
		t.Fatalf("project snapshot leaked tasks: %+v", body)
	}
	f.ledgerTurn("turn", "session-turn", f.at(-time.Hour), true, VerdictActed)
	for _, id := range []string{"in", "out", "none", "deleted", "dangling"} {
		f.callRow("turn", "move", id, true)
	}
	page, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "in"})
	if err != nil || len(page.Turns) != 1 || len(page.Turns[0].Calls) != 5 {
		t.Fatalf("allowed project digest unavailable: %+v %v", page, err)
	}
	for i, call := range page.Turns[0].Calls {
		if (call.Target != nil) != (i == 0) {
			t.Fatalf("digest exposed excluded target at %d: %+v", i, call)
		}
	}
	for _, id := range []string{"out", "none", "deleted", "dangling"} {
		if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: id}); !errors.Is(err, coordinator.ErrNotFound) {
			t.Fatalf("excluded task %s filter accepted: %v", id, err)
		}
	}
	f.watch.Projects.IncludeNoRepo = true
	_, body = f.snapshot(f.at(time.Hour))
	if body.TasksTotal != 2 {
		t.Fatalf("no-repository toggle included bound excluded tasks: %+v", body)
	}
	f.exec(`DELETE FROM repository_set_items WHERE repository_id = 'repo'`)
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "in"}); !errors.Is(err, coordinator.ErrNotFound) {
		t.Fatalf("removed set membership remained visible: %v", err)
	}
	f.watch.Projects.RepoIDs = []string{"repo"}
	for i := 0; i < snapshotTaskCap; i++ {
		id := fmt.Sprintf("outside-%03d", i)
		f.task(id, "wf", "step", "TODO", f.at(time.Hour), false)
		f.exec(`INSERT INTO task_repositories VALUES (?, 'foreign')`, id)
	}
	_, body = f.snapshot(f.at(2 * time.Hour))
	if body.TasksTotal != 2 || len(body.Tasks) != 2 {
		t.Fatalf("project filtering happened after snapshot cap: %+v", body)
	}
}

func TestLedgerProjectsBoundarySQLite(t *testing.T) { testLedgerProjectsBoundary(t, newFixture(t)) }
func TestLedgerProjectsBoundaryPostgres(t *testing.T) {
	testLedgerProjectsBoundary(t, newPostgresFixture(t))
}

func TestLedgerProjectsReadFailureReturnsNoPartialData(t *testing.T) {
	f := newFixture(t)
	f.watch = coordinator.WatchSet{All: true, Projects: watch.Scope{Selected: true, RepoIDs: []string{"repo"}}}
	f.ledgerTurn("turn", "session-turn", f.at(-time.Hour), true, VerdictActed)
	page, err := f.l.List(t.Context(), f.coord.ID, ListArgs{})
	if page != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing project tables yielded a partial page: %+v %v", page, err)
	}
	snapshot, err := BuildSnapshot(t.Context(), f.db, f.coord.ID, "ws-1", f.watch, f.now)
	if snapshot != nil || err == nil {
		t.Fatalf("missing project tables yielded a snapshot: %+v %v", snapshot, err)
	}
}
