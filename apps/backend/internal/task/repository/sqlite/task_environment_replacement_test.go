package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// seedRetainedFailedEnvironment leaves taskID with a failed environment that
// has physical evidence and two terminal sessions bound to it.
func seedRetainedFailedEnvironment(t *testing.T, repo *Repository, taskID, ownerID, siblingID string) *models.TaskEnvironment {
	t.Helper()
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, taskID)
	environmentID := seedFailedEnvironment(t, repo, taskID, ownerID)
	addWorktreeEvidence(t, repo, environmentID)
	if err := repo.UpdateTaskSessionState(ctx, ownerID, models.TaskSessionStateFailed, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: siblingID, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET task_environment_id = ?, state = ? WHERE id = ?`),
		environmentID, string(models.TaskSessionStateCompleted), siblingID); err != nil {
		t.Fatal(err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func closedReader(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "closed-reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	reader := sqlx.NewDb(conn, "sqlite3")
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return reader
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestReplacementReloadFailureReleasesOwnedReservation(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const taskID = "task-replacement-reload"
	retained := seedRetainedFailedEnvironment(t, repo, taskID, "session-replacement-owner", "session-replacement-sibling")

	// The transaction commits, then the reload on the read connection fails.
	reader := repo.ro
	repo.ro = closedReader(t)
	_, err := repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
		ID: "session-replacement-owner", TaskID: taskID, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateFailed,
	}, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal), TaskDirName: "replacement-reload"})
	repo.ro = reader
	if err == nil {
		t.Fatal("replacement returned success although its reload failed")
	}

	reservation, err := repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || reservation == nil {
		t.Fatalf("active reservation = %v, %v", reservation, err)
	}
	if reservation.ID == retained.ID || reservation.Status != models.TaskEnvironmentStatusFailed || reservation.MaterializationSessionID != "" {
		t.Fatalf("unpublished reservation = %s status %q owner %q, want a released failed replacement",
			reservation.ID, reservation.Status, reservation.MaterializationSessionID)
	}

	// A sibling still bound to the retired environment can now take the
	// released reservation instead of waiting for a materializer that never runs.
	selected, err := repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
		ID: "session-replacement-sibling", TaskID: taskID, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateCompleted,
	}, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal), TaskDirName: "replacement-reload"})
	if err != nil {
		var preparing *models.WorkspacePreparingError
		if errors.As(err, &preparing) {
			t.Fatalf("sibling replacement is blocked by the unpublished reservation: %v", err)
		}
		t.Fatal(err)
	}
	if selected.ID != reservation.ID || selected.Status != models.TaskEnvironmentStatusCreating ||
		selected.MaterializationSessionID != "session-replacement-sibling" {
		t.Fatalf("sibling replacement = %s status %q owner %q, want re-elected %s",
			selected.ID, selected.Status, selected.MaterializationSessionID, reservation.ID)
	}
}
