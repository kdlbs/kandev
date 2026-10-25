package coordinator

import (
	"context"
	"testing"
	"time"
)

func testDreamDecisionMetadataBoundary(t *testing.T, s *Store) {
	t.Helper()
	mustExec(t, s, `CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT)`)
	own := newTestCoordinator(t, s, "own-workspace")
	foreign := newTestCoordinator(t, s, "foreign-workspace")
	now := time.Now().UTC()
	for _, c := range []*Coordinator{own, foreign} {
		mustExec(t, s, `INSERT INTO coordinator_proposals (id,coordinator_id,workspace_id,status,spec_json,kind,created_at,updated_at) VALUES (?,?,?,'approved',?,'create_task',?,?)`, c.ID+"-proposal", c.ID, c.WorkspaceID, `{"title":"`+c.Name+` private title"}`, now, now)
		mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id,coordinator_id,kind,decision,decided_at,cost_subcents,graded_at) VALUES (?,?,'create_task','approved',?,123,?)`, c.ID+"-proposal", c.ID, now, now)
		mustExec(t, s, `INSERT INTO coordinator_feedback (id,coordinator_id,kind,proposal_id,user_id,proposal_kind,created_at) VALUES (?,?,'edited',?,'manager','create_task',?)`, c.ID+"-feedback", own.ID, c.ID+"-proposal", now)
	}
	decisions, err := s.DreamWindowDecisions(context.Background(), own.ID, now.Add(-time.Minute), now.Add(time.Minute), 20)
	if err != nil {
		t.Fatal(err)
	}
	positive := false
	for _, d := range decisions {
		if d.ProposalID == foreign.ID+"-proposal" && (d.Title != "" || d.CostSubcents != nil || d.EditedFields != "[]" || d.TaskResult != "") {
			t.Fatalf("foreign metadata crossed coordinator boundary: %+v", d)
		}
		if d.ProposalID == own.ID+"-proposal" && d.Decision == "edited" && d.Title == own.Name+" private title" && d.CostSubcents != nil {
			positive = true
		}
	}
	if !positive {
		t.Fatal("same coordinator override lost its evidence")
	}
}
func TestDreamDecisionMetadataBoundarySQLite(t *testing.T) {
	testDreamDecisionMetadataBoundary(t, newTestStore(t))
}
func TestDreamDecisionMetadataBoundaryPostgres(t *testing.T) {
	testDreamDecisionMetadataBoundary(t, newTestStorePostgres(t))
}
