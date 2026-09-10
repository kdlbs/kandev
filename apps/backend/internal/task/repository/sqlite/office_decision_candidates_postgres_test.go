package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListOfficeDecisionWaitCandidates is the PostgreSQL twin of the
// SQLite candidate-enumeration tests. The query is dialect-sensitive in two
// places the SQLite run cannot exercise: excludeConfigModePredicate expands to
// a JSON expression that differs per driver, and the office-identity EXISTS
// subquery joins workspaces on a nullable column. A regression in either is a
// query that errors on every tick in production while every SQLite test stays
// green — the detector would then surface nothing and look healthy.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListOfficeDecisionWaitCandidates(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	const workspaceID = "ws-pg-decision-wait"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "pg-decision-wait", OwnerID: "u-1",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, workspaceID)
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}

	seatRow(t, repo, "pg-seat-review", "pg-step-review", "", "reviewer", true)
	seatRow(t, repo, "pg-seat-plain", "pg-step-plain", "", "reviewer", false)

	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "pg-t-waiting", "pg-step-review", "", "IN_PROGRESS")
	seedCandidateTask(t, repo, workspaceID, "wf-pg-kanban", "pg-t-kanban", "pg-step-review", "", "IN_PROGRESS")
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "pg-t-no-seat", "pg-step-plain", "", "IN_PROGRESS")

	quiet := time.Now().UTC().Add(-2 * time.Hour)
	for _, id := range []string{"pg-t-waiting", "pg-t-kanban", "pg-t-no-seat"} {
		stampTaskQuiet(t, repo, id, quiet)
	}

	got := candidateIDs(t, repo, time.Now().UTC().Add(-time.Hour))
	if !hasID(got, "pg-t-waiting") {
		t.Errorf("pg-t-waiting missing from candidates %v", got)
	}
	for _, unwanted := range []string{"pg-t-kanban", "pg-t-no-seat"} {
		if hasID(got, unwanted) {
			t.Errorf("candidate %q must not be selected; got %v", unwanted, got)
		}
	}
}
