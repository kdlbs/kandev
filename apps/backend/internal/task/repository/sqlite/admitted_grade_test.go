package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-SESSION-CEILING-005.2, AC-AGENTS-SESSION-CEILING-005.3
func TestAdmittedGradeSnapshotSurvivesOrdinarySessionUpdates(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var repo *Repository
			if backend == "postgres" {
				repo, _, _ = newTaskPostgresRepoPair(t)
			} else {
				repo = newRepoForEntityTests(t)
			}
			testAdmittedGradeSnapshot(t, repo)
		})
	}
}

func testAdmittedGradeSnapshot(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-admitted-grade"
	seedMaterializationClaimTask(t, repo, taskID)
	for _, id := range []string{"known-grade", "legacy-grade"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: taskID, State: models.TaskSessionStateCreated}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetAdmittedExecutionGrade(ctx, "known-grade", "work"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"known-grade", "legacy-grade"} {
		if err := repo.UpdateTaskSessionState(ctx, id, models.TaskSessionStateStarting, ""); err != nil {
			t.Fatal(err)
		}
	}
	session, err := repo.GetTaskSession(ctx, "known-grade")
	if err != nil {
		t.Fatal(err)
	}
	session.Metadata = map[string]interface{}{"note": "ordinary session edit"}
	session.AdmittedExecutionGrade, session.AdmittedExecutionGradeKnown = "control", false
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"known-grade", "legacy-grade"} {
		if err := repo.SetAdmittedExecutionGrade(ctx, id, "control"); err == nil {
			t.Fatalf("active session %s accepted a replacement grade", id)
		}
	}
	rows, err := repo.ListAdmittedExecutions(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("admitted executions = %+v, %v; want two sessions", rows, err)
	}
	for _, row := range rows {
		if row.ID == "known-grade" && (!row.GradeKnown || row.Grade != "work") {
			t.Errorf("ordinary edit changed admitted snapshot: %+v", row)
		}
		if row.ID == "legacy-grade" && (row.GradeKnown || row.Grade != "") {
			t.Errorf("legacy session lost its unknown grade: %+v", row)
		}
	}
}
