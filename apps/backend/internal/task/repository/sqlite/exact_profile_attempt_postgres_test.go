package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// The holder replaces the assignment only after the production attempt method
// is waiting on its FOR UPDATE lock. A lockless validation read would retain
// the old assignment and write a stale binding or receipt after this commit.
func TestPostgresExactProfileAttemptRereadsAssignmentAfterReplacement(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(context.Context, *Repository, *models.ExactProfileLaunchAttemptBinding, *models.ExactProfileLaunchReceipt) (bool, error)
	}{
		{name: "bind", run: func(ctx context.Context, repo *Repository, binding *models.ExactProfileLaunchAttemptBinding, _ *models.ExactProfileLaunchReceipt) (bool, error) {
			return repo.BindExactProfileLaunchAttempt(ctx, binding)
		}},
		{name: "receipt", run: func(ctx context.Context, repo *Repository, binding *models.ExactProfileLaunchAttemptBinding, receipt *models.ExactProfileLaunchReceipt) (bool, error) {
			return repo.RecordExactProfileLaunchReceiptForAttempt(ctx, binding, receipt)
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			repoA, repoB, observer := newTaskPostgresRepoPair(t)
			ctx := context.Background()
			assignment := seedPostgresExactProfileAttempt(t, repoA, operation.name)
			binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "exact-pg-session-" + operation.name, ExecutionID: "exact-pg-execution-" + operation.name, AttemptID: "exact-pg-attempt-" + operation.name, SessionIncarnationID: "exact-pg-incarnation-" + operation.name, AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
			if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: binding.SessionID, TaskID: binding.TaskID, QueueIncarnationID: binding.SessionIncarnationID, State: models.TaskSessionStateCreated}); err != nil {
				t.Fatal(err)
			}
			if operation.name == "receipt" {
				if changed, err := repoA.BindExactProfileLaunchAttempt(ctx, binding); err != nil || !changed {
					t.Fatalf("seed binding = (%v, %v)", changed, err)
				}
			}
			holder, err := repoA.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback() }()
			if _, err := holder.ExecContext(ctx, repoA.db.Rebind(`SELECT 1 FROM task_exact_profile_assignments WHERE task_id = ? FOR UPDATE`), assignment.TaskID); err != nil {
				t.Fatal(err)
			}

			writerPID := pgBackendPID(t, repoB.db)
			done := make(chan struct{})
			var writerErr error
			go func() {
				defer close(done)
				receipt := &models.ExactProfileLaunchReceipt{TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID, ProfileRevision: binding.ProfileRevision, Generation: binding.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
				_, writerErr = operation.run(ctx, repoB, binding, receipt)
			}()
			if err := waitForPostgresLock(ctx, observer, writerPID, done); err != nil {
				t.Fatal(err)
			}
			nextRevision := assignment.ProfileRevision.Add(time.Second)
			if _, err := holder.ExecContext(ctx, repoA.db.Rebind(`UPDATE task_exact_profile_assignments SET profile_revision = ?, generation = ? WHERE task_id = ? AND generation = ?`), nextRevision, assignment.Generation+1, assignment.TaskID, assignment.Generation); err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("attempt operation did not finish after replacement commit")
			}
			if !errors.Is(writerErr, models.ErrExactProfileAssignmentGeneration) {
				t.Fatalf("operation error = %v, want assignment generation rejection", writerErr)
			}
			if operation.name == "receipt" {
				receipt, err := repoA.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
				if err != nil || receipt != nil {
					t.Fatalf("stale receipt = %#v, %v", receipt, err)
				}
			} else {
				var count int
				if err := repoA.db.GetContext(ctx, &count, repoA.db.Rebind(`SELECT COUNT(*) FROM task_exact_profile_launch_attempt_bindings WHERE task_id = ? AND session_id = ?`), binding.TaskID, binding.SessionID); err != nil || count != 0 {
					t.Fatalf("stale binding count = %d, %v", count, err)
				}
			}
		})
	}
}

func seedPostgresExactProfileAttempt(t *testing.T, repo *Repository, suffix string) *models.ExactProfileAssignment {
	t.Helper()
	ctx := context.Background()
	workspaceID := "exact-pg-workspace-" + suffix
	taskID := "exact-pg-task-" + suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Exact PostgreSQL " + suffix}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Exact PostgreSQL " + suffix, Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	assignment := &models.ExactProfileAssignment{TaskID: taskID, WorkspaceID: workspaceID, AgentProfileID: "exact-pg-profile-" + suffix, ProfileRevision: time.Now().UTC().Round(0), Generation: 1}
	if changed, err := repo.AssignExactProfileAssignment(ctx, assignment); err != nil || !changed {
		t.Fatalf("assign = (%v, %v)", changed, err)
	}
	return assignment
}
