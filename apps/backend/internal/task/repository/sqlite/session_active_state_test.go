package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestActiveSessionStateMatchesSQLFilter cross-checks GetActiveTaskSessionByTaskID's
// SQL state filter against models.IsTaskLookupActiveSessionState for every state in
// models.AllTaskSessionStates, the package's single canonical state list. Editing
// the SQL filter without updating the predicate (or vice versa) fails this test
// instead of only a stale comment — but only for states already present in
// AllTaskSessionStates: Go does not enforce switch/slice exhaustiveness, so a new
// TaskSessionState constant added to models.go without also being added to
// AllTaskSessionStates is not caught by this test (or by any linter — the
// exhaustive linter is not enabled in this repo).
func TestActiveSessionStateMatchesSQLFilter(t *testing.T) {
	for _, state := range models.AllTaskSessionStates {
		t.Run(string(state), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := "task-active-state-" + string(state)
			sessionID := "session-active-state-" + string(state)

			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Active state check"}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID:     sessionID,
				TaskID: taskID,
				State:  state,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}

			got, err := repo.GetActiveTaskSessionByTaskID(ctx, taskID)
			wantActive := models.IsTaskLookupActiveSessionState(state)

			if wantActive {
				if err != nil {
					t.Fatalf("GetActiveTaskSessionByTaskID(%q) = %v, want session (state %q is active)", taskID, err, state)
				}
				if got == nil || got.ID != sessionID {
					t.Fatalf("GetActiveTaskSessionByTaskID(%q) returned wrong session: %+v", taskID, got)
				}
				return
			}

			if !errors.Is(err, models.ErrTaskSessionNotFound) {
				t.Fatalf("GetActiveTaskSessionByTaskID(%q) = (%v, %v), want ErrTaskSessionNotFound (state %q is not active)", taskID, got, err, state)
			}
		})
	}
}

func TestActiveSessionLookupExcludesArchivedLegacyRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-archived-active", Title: "Archived active"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-archived-active", TaskID: "task-archived-active", State: models.TaskSessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET archived_at = ? WHERE id = ?`, time.Now().UTC(), "session-archived-active"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetActiveTaskSessionByTaskID(ctx, "task-archived-active")
	if !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("archived active lookup = %+v, %v", got, err)
	}
}
