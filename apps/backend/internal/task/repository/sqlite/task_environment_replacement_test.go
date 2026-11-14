package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_environments SET executor_type = ? WHERE id = ?`),
		string(models.ExecutorTypeWorktree), environmentID); err != nil {
		t.Fatal(err)
	}
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

func seedActiveReplacementReservation(
	t *testing.T,
	repo *Repository,
	taskID, environmentID, ownerID string,
	ownerState models.TaskSessionState,
	claimAge time.Duration,
	physical, live bool,
) *models.TaskEnvironment {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	environment := &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: ownerID, TaskDirName: "replacement-successor", CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatal(err)
	}
	if ownerID != "" && ownerState != "" {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: ownerID, TaskID: taskID, TaskEnvironmentID: environmentID, State: ownerState,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if physical {
		addWorktreeEvidence(t, repo, environmentID)
	}
	if live {
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "running-" + ownerID, SessionID: ownerID, TaskID: taskID,
			ExecutorID: "executor-" + ownerID, AgentExecutionID: "execution-" + ownerID,
			Status: models.ExecutorRunningStatusRunning,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if claimAge > 0 {
		ageTaskEnvironment(t, repo, environmentID, claimAge)
	}
	return environment
}

func taskEnvironmentHasRetiredAt(t *testing.T, repo *Repository) bool {
	t.Helper()
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := repo.columnExists(tx, "task_environments", "retired_at")
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

func TestTaskEnvironmentRetiredAtIsAddedByRunMigrations(t *testing.T) {
	repo := newRepoForEntityTests(t)
	if !taskEnvironmentHasRetiredAt(t, repo) {
		t.Fatal("initialized task environments do not have retired_at")
	}
	if _, err := repo.db.ExecContext(context.Background(), `DROP INDEX IF EXISTS uniq_task_environments_task_id`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(context.Background(), `ALTER TABLE task_environments DROP COLUMN retired_at`); err != nil {
		t.Fatalf("remove retired_at to simulate the schema before its migration: %v", err)
	}
	if taskEnvironmentHasRetiredAt(t, repo) {
		t.Fatal("retired_at remained after simulating the pre-migration schema")
	}
	if err := repo.runMigrations(context.Background()); err != nil {
		t.Fatalf("run retired_at migration: %v", err)
	}
	if !taskEnvironmentHasRetiredAt(t, repo) {
		t.Fatal("runMigrations did not add retired_at")
	}
	if err := repo.runMigrations(context.Background()); err != nil {
		t.Fatalf("replay retired_at migration: %v", err)
	}
}

func TestReplacementRetrySettlesAbandonedSuccessorClaim(t *testing.T) {
	cases := []struct {
		name             string
		ownerID          string
		ownerState       models.TaskSessionState
		claimAge         time.Duration
		physical         bool
		live             bool
		wantPreparing    bool
		wantFresh        bool
		failedAfterClaim bool
	}{
		{name: "missing owner", ownerID: "missing-successor-owner"},
		{name: "terminal owner", ownerID: "terminal-successor-owner", ownerState: models.TaskSessionStateFailed, claimAge: 24 * time.Hour},
		{name: "owner failed after claim", ownerID: "failed-successor-owner", ownerState: models.TaskSessionStateCreated, failedAfterClaim: true},
		{name: "stale owner without runtime", ownerID: "stale-successor-owner", ownerState: models.TaskSessionStateCreated, claimAge: 24 * time.Hour},
		{name: "terminal owner with physical evidence", ownerID: "physical-successor-owner", ownerState: models.TaskSessionStateFailed, claimAge: 24 * time.Hour, physical: true, wantFresh: true},
		{name: "young owner", ownerID: "young-successor-owner", ownerState: models.TaskSessionStateCreated, wantPreparing: true},
		{name: "young terminal owner", ownerID: "young-terminal-successor-owner", ownerState: models.TaskSessionStateFailed, wantPreparing: true},
		{name: "live stale owner", ownerID: "live-successor-owner", ownerState: models.TaskSessionStateRunning, claimAge: 24 * time.Hour, live: true, wantPreparing: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			taskID := "task-replacement-claim-" + tc.name
			retained := seedRetainedFailedEnvironment(t, repo, taskID, "session-old-owner", "session-historical-retry")
			if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
				`UPDATE task_environments SET retired_at = ? WHERE id = ?`), time.Now().UTC(), retained.ID); err != nil {
				t.Fatal(err)
			}
			retained, err := repo.GetTaskEnvironment(context.Background(), retained.ID)
			if err != nil {
				t.Fatal(err)
			}
			successorID := "environment-successor-" + tc.name
			successor := seedActiveReplacementReservation(t, repo, taskID, successorID, tc.ownerID,
				tc.ownerState, tc.claimAge, tc.physical, tc.live)
			if tc.failedAfterClaim {
				if err := repo.UpdateTaskSessionState(context.Background(), tc.ownerID, models.TaskSessionStateFailed, ""); err != nil {
					t.Fatal(err)
				}
			}

			selected, err := repo.ReserveTaskEnvironmentReplacement(context.Background(), retained, &models.TaskSession{
				ID: "session-historical-retry", TaskID: taskID, TaskEnvironmentID: retained.ID,
				State: models.TaskSessionStateCompleted,
			}, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree), TaskDirName: "replacement-next"})
			if tc.wantPreparing {
				if !errors.Is(err, models.ErrWorkspacePreparing) {
					t.Fatalf("replacement error = %v, want workspace preparing", err)
				}
				current, loadErr := repo.GetTaskEnvironment(context.Background(), successor.ID)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if current.Status != models.TaskEnvironmentStatusCreating || current.MaterializationSessionID != tc.ownerID {
					t.Fatalf("successor = status %q owner %q, want unchanged creating owner %q",
						current.Status, current.MaterializationSessionID, tc.ownerID)
				}
				return
			}
			if err != nil {
				t.Fatalf("replacement after abandoned successor claim: %v", err)
			}
			if tc.wantFresh {
				if selected.ID == successor.ID {
					t.Fatalf("replacement reused physical successor %q", successor.ID)
				}
				retired, loadErr := repo.GetTaskEnvironment(context.Background(), successor.ID)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if !retired.IsRetired() || len(retired.Repos) != 1 {
					t.Fatalf("physical successor = retired %v, repos %d; want retained evidence", retired.IsRetired(), len(retired.Repos))
				}
				return
			}
			if selected.ID != successor.ID || selected.Status != models.TaskEnvironmentStatusCreating ||
				selected.MaterializationSessionID != "session-historical-retry" {
				t.Fatalf("replacement = %s status %q owner %q, want successor adopted by historical retry",
					selected.ID, selected.Status, selected.MaterializationSessionID)
			}
		})
	}
}

func TestWorkspaceBindingDoesNotRetireUnsupportedExecutorEnvironment(t *testing.T) {
	cases := []struct {
		name              string
		oldExecutorType   models.ExecutorType
		newExecutorType   models.ExecutorType
		workspaceEvidence bool
	}{
		{name: "remote to worktree", oldExecutorType: models.ExecutorTypeSSH, newExecutorType: models.ExecutorTypeWorktree},
		{name: "worktree to remote", oldExecutorType: models.ExecutorTypeWorktree, newExecutorType: models.ExecutorTypeSSH, workspaceEvidence: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			taskID := "task-binding-scope-" + tc.name
			seedMaterializationClaimTask(t, repo, taskID)
			owner := seedCreatingOwner(t, repo, taskID, "session-binding-scope-owner", models.TaskSessionStateCreated)
			if _, err := repo.FailTaskEnvironmentMaterialization(context.Background(), owner.ID); err != nil {
				t.Fatal(err)
			}
			if err := repo.UpdateTaskSessionState(context.Background(), owner.ID, models.TaskSessionStateFailed, ""); err != nil {
				t.Fatal(err)
			}
			if tc.workspaceEvidence {
				addWorktreeEvidence(t, repo, owner.TaskEnvironmentID)
			} else if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
				`UPDATE task_environments SET container_id = ? WHERE id = ?`), "retained-container", owner.TaskEnvironmentID); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
				`UPDATE task_environments SET executor_type = ? WHERE id = ?`), string(tc.oldExecutorType), owner.TaskEnvironmentID); err != nil {
				t.Fatal(err)
			}

			session := &models.TaskSession{ID: "session-binding-scope-retry", TaskID: taskID}
			err := repo.CreateTaskSessionWithWorkspaceBinding(context.Background(), session, &models.TaskEnvironment{
				TaskID: taskID, ExecutorType: string(tc.newExecutorType), TaskDirName: "unsupported-replacement",
			})
			if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
				t.Fatalf("replacement error = %v, want reset required", err)
			}
			current, loadErr := repo.GetTaskEnvironmentByTaskID(context.Background(), taskID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if current.ID != owner.TaskEnvironmentID || current.IsRetired() || current.ExecutorType != string(tc.oldExecutorType) {
				t.Fatalf("canonical environment = %s retired %v type %q, want original unretired %q",
					current.ID, current.IsRetired(), current.ExecutorType, tc.oldExecutorType)
			}
			if !tc.workspaceEvidence && current.ContainerID != "retained-container" {
				t.Fatalf("container inventory = %q, want retained-container", current.ContainerID)
			}
			if _, err := repo.GetTaskSession(context.Background(), session.ID); err == nil {
				t.Fatal("refused replacement persisted the new session")
			}
		})
	}
}

func TestReplacementForLiveSessionKeepsResetGuidance(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-replacement-live-session"
		sessionID = "session-replacement-live"
	)
	retained := seedRetainedFailedEnvironment(t, repo, taskID, sessionID, "session-replacement-live-sibling")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ReserveTaskEnvironmentReplacement(ctx, retained, session, &models.TaskEnvironment{
		TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree), TaskDirName: "replacement-live-session",
	})
	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		t.Fatalf("replacement for a live session = %v, want reset required guidance", err)
	}
	current, err := repo.GetTaskEnvironment(ctx, retained.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.IsRetired() || current.Status != models.TaskEnvironmentStatusFailed {
		t.Fatalf("live session environment = retired %v status %q, want retained failed environment",
			current.IsRetired(), current.Status)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestReplacementReloadFailureReleasesOwnedReservation(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		testReplacementReloadFailureReleasesOwnedReservation(t, newRepoForEntityTests(t))
	})
	t.Run("postgres", func(t *testing.T) {
		repo, _, _ := newTaskPostgresRepoPair(t)
		testReplacementReloadFailureReleasesOwnedReservation(t, repo)
	})
}

func testReplacementReloadFailureReleasesOwnedReservation(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-replacement-reload"
	retained := seedRetainedFailedEnvironment(t, repo, taskID, "session-replacement-owner", "session-replacement-sibling")

	// The transaction commits, then the reload on the read connection fails.
	reader := repo.ro
	repo.ro = closedReader(t)
	_, err := repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
		ID: "session-replacement-owner", TaskID: taskID, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateFailed,
	}, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree), TaskDirName: "replacement-reload"})
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
	}, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree), TaskDirName: "replacement-reload"})
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
