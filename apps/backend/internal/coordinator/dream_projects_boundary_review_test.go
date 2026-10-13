package coordinator

import (
	"testing"
	"time"
)

func testDreamProjectsBoundary(t *testing.T, s *Store) {
	t.Helper()
	f := newWakeFixture(t, s)
	seedClaimProjectTables(t, s)
	mustExec(t, s, `INSERT INTO repositories (id, workspace_id) VALUES ('repo', 'ws-1'), ('foreign', 'foreign-workspace')`)
	projectScope(t, s, f.c.ID, false, repoEntry("repo"))
	now := time.Now().UTC()
	for _, id := range []string{"in", "out", "none"} {
		f.addOwn(t, id, "workflow")
		mustExec(t, s, `UPDATE coordinator_proposals SET spec_json = ? WHERE task_id = ?`, `{"title":"`+id+` private title"}`, id)
		mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id,coordinator_id,kind,decision,decided_at,graded_at)
			SELECT id,coordinator_id,'create_task','approved',?,? FROM coordinator_proposals WHERE task_id = ?`, now, now, id)
	}
	mustExec(t, s, `INSERT INTO task_repositories VALUES ('link-in','in','repo'), ('link-out','out','foreign')`)
	decisions, err := s.DreamWindowDecisions(t.Context(), f.c.ID, now.Add(-time.Minute), now.Add(time.Minute), 20)
	if err != nil || len(decisions) != 1 || decisions[0].Title != "in private title" {
		t.Fatalf("dream evidence leaked excluded project: %+v %v", decisions, err)
	}
	mustExec(t, s, `UPDATE coordinators SET include_no_repository = 1 WHERE id = ?`, f.c.ID)
	decisions, err = s.DreamWindowDecisions(t.Context(), f.c.ID, now.Add(-time.Minute), now.Add(time.Minute), 20)
	if err != nil || len(decisions) != 2 {
		t.Fatalf("no-repository decision missing or foreign included: %+v %v", decisions, err)
	}
	mustExec(t, s, `UPDATE repositories SET workspace_id = 'foreign-workspace' WHERE id = 'repo'`)
	decisions, err = s.DreamWindowDecisions(t.Context(), f.c.ID, now.Add(-time.Minute), now.Add(time.Minute), 20)
	if err != nil || len(decisions) != 1 || decisions[0].Title != "none private title" {
		t.Fatalf("moved repository remained in evidence: %+v %v", decisions, err)
	}
	mustExec(t, s, `DROP TABLE repositories`)
	decisions, err = s.DreamWindowDecisions(t.Context(), f.c.ID, now.Add(-time.Minute), now.Add(time.Minute), 20)
	if decisions != nil || err == nil {
		t.Fatalf("project read failure yielded partial dream evidence: %+v %v", decisions, err)
	}
}

func TestDreamProjectsBoundarySQLite(t *testing.T) { testDreamProjectsBoundary(t, newTestStore(t)) }
func TestDreamProjectsBoundaryPostgres(t *testing.T) {
	testDreamProjectsBoundary(t, newTestStorePostgres(t))
}
