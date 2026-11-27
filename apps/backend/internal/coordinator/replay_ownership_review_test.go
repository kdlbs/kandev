package coordinator

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

func testReplayOwnershipBoundary(t *testing.T, s *Store) {
	t.Helper()
	own := newTestCoordinator(t, s, "ws-1")
	foreign := newTestCoordinator(t, s, "foreign-workspace")
	mustExec(t, s, `CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, title TEXT);
		INSERT INTO tasks VALUES ('own-task','ws-1','workflow','own title'), ('foreign-task','foreign-workspace','workflow','private title')`)
	now := time.Now().UTC()
	for _, c := range []*Coordinator{own, foreign} {
		mustExec(t, s, `INSERT INTO coordinator_proposals (id,coordinator_id,workspace_id,status,spec_json,kind,created_at,updated_at)
			VALUES (?,?,?,'approved','{"title":"private proposal"}','create_task',?,?)`, c.ID+"-proposal", c.ID, c.WorkspaceID, now, now)
	}
	cases := NewReplayCases(s)
	if _, err := cases.Proposal(t.Context(), own.ID, foreign.ID+"-proposal"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("foreign proposal crossed replay ownership: %v", err)
	}
	if _, err := cases.Proposal(t.Context(), own.ID, own.ID+"-proposal"); err != nil {
		t.Fatalf("own proposal missing: %v", err)
	}
	if _, err := cases.TaskTitle(t.Context(), own.ID, "foreign-task"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("foreign title crossed replay workspace: %v", err)
	}
	if title, err := cases.TaskTitle(t.Context(), own.ID, "own-task"); err != nil || title != "own title" {
		t.Fatalf("own title missing: %q %v", title, err)
	}
	mustExec(t, s, `UPDATE tasks SET workspace_id = 'foreign-workspace' WHERE id = 'own-task'`)
	if _, err := cases.TaskTitle(t.Context(), own.ID, "own-task"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("moved title crossed replay workspace: %v", err)
	}
	mustExec(t, s, `UPDATE tasks SET workspace_id = 'ws-1' WHERE id = 'own-task'`)
	seedClaimProjectTables(t, s)
	mustExec(t, s, `INSERT INTO repositories (id, workspace_id) VALUES ('repo','ws-1')`)
	projectScope(t, s, own.ID, false, repoEntry("repo"))
	if _, err := cases.TaskTitle(t.Context(), own.ID, "own-task"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("repository-less title leaked with toggle off: %v", err)
	}
	mustExec(t, s, `INSERT INTO task_repositories VALUES ('link','own-task','repo')`)
	if title, err := cases.TaskTitle(t.Context(), own.ID, "own-task"); err != nil || title != "own title" {
		t.Fatalf("allowed repository title missing: %q %v", title, err)
	}
	mustExec(t, s, `INSERT INTO coordinator_turns (id,coordinator_id,session_id,session_turn_id,"trigger",started_at)
		VALUES ('turn',?,'session','session-turn','wake',?)`, own.ID, now)
	mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id,coordinator_id,turn_id,kind,decision,decided_at,graded_at)
		VALUES (?,?,'turn','create_task','approved',?,?)`, foreign.ID+"-proposal", foreign.ID, now, now)
	turns, err := cases.SelectTurns(t.Context(), own.ID, now.Add(time.Minute))
	if err != nil || len(turns) != 0 {
		t.Fatalf("foreign outcome admitted an owned replay turn: %+v %v", turns, err)
	}
	mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id,coordinator_id,turn_id,kind,decision,decided_at,graded_at)
		VALUES (?,?,'turn','create_task','approved',?,?)`, own.ID+"-proposal", own.ID, now, now)
	turns, err = cases.SelectTurns(t.Context(), own.ID, now.Add(time.Minute))
	if err != nil || len(turns) != 1 || len(turns[0].Outcomes) != 1 || turns[0].Outcomes[0].ProposalID != own.ID+"-proposal" {
		t.Fatalf("owned replay lost outcome or adopted foreign metadata: %+v %v", turns, err)
	}
}

func TestReplayOwnershipBoundarySQLite(t *testing.T) { testReplayOwnershipBoundary(t, newTestStore(t)) }
func TestReplayOwnershipBoundaryPostgres(t *testing.T) {
	testReplayOwnershipBoundary(t, newTestStorePostgres(t))
}
