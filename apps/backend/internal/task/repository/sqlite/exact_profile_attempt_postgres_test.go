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
			binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "exact-pg-session-" + operation.name, ExecutionID: "exact-pg-execution-" + operation.name, AttemptID: "exact-pg-attempt-" + operation.name, SessionIncarnationID: "exact-pg-incarnation-" + operation.name, AgentProfileID: assignment.AgentProfileID, Model: "model-exact", ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
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
				receipt := &models.ExactProfileLaunchReceipt{TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID, ProfileRevision: binding.ProfileRevision, Generation: binding.Generation, Model: binding.Model, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
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

func TestPostgresLegacyExactReceiptLookupSerializesWithSuccessorBind(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	assignment := seedPostgresExactProfileAttempt(t, repoA, "legacy-fallback")
	const sessionID = "exact-pg-legacy-fallback-session"
	const incarnationID = "exact-pg-legacy-fallback-incarnation"
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: assignment.TaskID, QueueIncarnationID: incarnationID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	legacy := &models.ExactProfileLaunchReceipt{TaskID: assignment.TaskID, SessionID: sessionID, AgentProfileID: assignment.AgentProfileID, Model: "model-exact", ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repoA.RecordExactProfileLaunchReceipt(ctx, legacy); err != nil || !changed {
		t.Fatalf("legacy receipt = (%v, %v)", changed, err)
	}
	lookupLocked := make(chan struct{})
	releaseLookup := make(chan struct{})
	repoB.exactProfileReceiptLegacyFallbackHook = func() { close(lookupLocked); <-releaseLookup }
	lookupDone := make(chan struct{})
	var lookupReceipt *models.ExactProfileLaunchReceipt
	var lookupErr error
	go func() {
		defer close(lookupDone)
		lookupReceipt, lookupErr = repoB.GetExactProfileLaunchReceipt(ctx, assignment.TaskID, sessionID)
	}()
	select {
	case <-lookupLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("legacy lookup did not reach locked fallback")
	}
	binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: sessionID, ExecutionID: "exact-pg-legacy-fallback-execution", AttemptID: "exact-pg-legacy-fallback-attempt", SessionIncarnationID: incarnationID, AgentProfileID: assignment.AgentProfileID, Model: "model-exact", ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	bindPID := pgBackendPID(t, repoA.db)
	bindDone := make(chan struct{})
	var bindErr error
	go func() { defer close(bindDone); _, bindErr = repoA.BindExactProfileLaunchAttempt(ctx, binding) }()
	if err := waitForPostgresLock(ctx, observer, bindPID, bindDone); err != nil {
		t.Fatal(err)
	}
	close(releaseLookup)
	select {
	case <-lookupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("legacy lookup did not finish")
	}
	if lookupErr != nil || lookupReceipt == nil || lookupReceipt.Outcome != legacy.Outcome {
		t.Fatalf("legacy lookup = %#v, %v", lookupReceipt, lookupErr)
	}
	select {
	case <-bindDone:
	case <-time.After(5 * time.Second):
		t.Fatal("successor bind did not finish")
	}
	if bindErr != nil {
		t.Fatalf("successor bind: %v", bindErr)
	}
	if receipt, err := repoA.GetExactProfileLaunchReceipt(ctx, assignment.TaskID, sessionID); err != nil || receipt != nil {
		t.Fatalf("successor inherited legacy receipt = %#v, %v", receipt, err)
	}
}

func TestPostgresCurrentExactReceiptLookupSerializesWithSuccessorAssignment(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	assignment := seedPostgresExactProfileAttempt(t, repoA, "current-lookup")
	binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "exact-pg-current-lookup-session", ExecutionID: "exact-pg-current-lookup-execution", AttemptID: "exact-pg-current-lookup-attempt", SessionIncarnationID: "exact-pg-current-lookup-incarnation", AgentProfileID: assignment.AgentProfileID, Model: "model-exact", ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: binding.SessionID, TaskID: binding.TaskID, QueueIncarnationID: binding.SessionIncarnationID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	if changed, err := repoA.BindExactProfileLaunchAttempt(ctx, binding); err != nil || !changed {
		t.Fatalf("bind = (%v, %v)", changed, err)
	}
	oldReceipt := &models.ExactProfileLaunchReceipt{TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID, ProfileRevision: binding.ProfileRevision, Generation: binding.Generation, Model: binding.Model, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repoA.RecordExactProfileLaunchReceiptForAttempt(ctx, binding, oldReceipt); err != nil || !changed {
		t.Fatalf("old receipt = (%v, %v)", changed, err)
	}
	locked, release := make(chan struct{}), make(chan struct{})
	repoB.exactProfileReceiptCurrentLookupHook = func() { close(locked); <-release }
	lookupDone := make(chan struct{})
	var lookup *models.ExactProfileLaunchReceipt
	var lookupErr error
	go func() {
		defer close(lookupDone)
		lookup, lookupErr = repoB.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("current lookup did not reach lock boundary")
	}
	next := *assignment
	next.Generation++
	next.ProfileRevision = next.ProfileRevision.Add(time.Second)
	writerPID := pgBackendPID(t, repoA.db)
	replaceDone := make(chan struct{})
	var replaceErr error
	go func() { defer close(replaceDone); _, replaceErr = repoA.AssignExactProfileAssignment(ctx, &next) }()
	if err := waitForPostgresLock(ctx, observer, writerPID, replaceDone); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-lookupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("current lookup did not finish")
	}
	if lookupErr != nil || lookup == nil || lookup.Generation != assignment.Generation {
		t.Fatalf("lookup = %#v, %v", lookup, lookupErr)
	}
	select {
	case <-replaceDone:
	case <-time.After(5 * time.Second):
		t.Fatal("successor assignment did not finish")
	}
	if replaceErr != nil {
		t.Fatalf("successor assignment: %v", replaceErr)
	}
	if receipt, err := repoA.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID); err != nil || receipt != nil {
		t.Fatalf("successor inherited predecessor receipt = %#v, %v", receipt, err)
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
