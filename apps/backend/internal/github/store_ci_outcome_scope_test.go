package github

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type ciOutcomeScopeSnapshot struct {
	legacyOptionRows int
	legacyOptions    []TaskCIOptions
	prOptions        []*TaskPRAutomationOptions
	states           []*TaskCIPRAutomationState
}

func snapshotCIAutoFixScope(t *testing.T, store *Store, taskID string) ciOutcomeScopeSnapshot {
	t.Helper()
	var legacyOptionRows int
	if err := store.db.GetContext(context.Background(), &legacyOptionRows,
		"SELECT COUNT(*) FROM github_task_ci_options WHERE task_id = ?", taskID); err != nil {
		t.Fatalf("count legacy options: %v", err)
	}
	var legacyOptions []TaskCIOptions
	if err := store.ro.SelectContext(context.Background(), &legacyOptions,
		store.ro.Rebind("SELECT * FROM github_task_ci_options WHERE task_id = ?"), taskID); err != nil {
		t.Fatalf("list legacy options: %v", err)
	}
	prOptions, err := store.ListTaskPRAutomationOptions(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list PR options: %v", err)
	}
	states, err := store.ListTaskCIPRStates(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list PR states: %v", err)
	}
	return ciOutcomeScopeSnapshot{
		legacyOptionRows: legacyOptionRows,
		legacyOptions:    legacyOptions,
		prOptions:        prOptions,
		states:           states,
	}
}

func requireCIAutoFixScopeUnchanged(t *testing.T, before, after ciOutcomeScopeSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("auto-fix state changed after rejected report:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

// @covers AC-UI-CI-PR-AUTOMATION-001.10
// @covers AC-UI-CI-PR-AUTOMATION-001.17
func TestStorePRAutoFixOutcomeOrdinaryTurnHasNoSideEffects(t *testing.T) {
	ctx := context.Background()
	report := func(store *Store, taskID, sessionID, turnID string) error {
		return store.ReportTaskCIAutoFixOutcome(ctx, TaskCIAutoFixOutcomeReport{
			TaskID: taskID, SessionID: sessionID, TurnID: turnID,
			Outcome: TaskCIAutoFixOutcomeBlocked, Summary: "ordinary PR work",
		})
	}

	t.Run("absent settings", func(t *testing.T) {
		store := newTestStore(t)
		before := snapshotCIAutoFixScope(t, store, "task-absent")
		if err := report(store, "task-absent", "session-current", "turn-current"); !errors.Is(err, ErrTaskCIAutoFixAttemptNotFound) {
			t.Fatalf("report error = %v, want %v", err, ErrTaskCIAutoFixAttemptNotFound)
		}
		after := snapshotCIAutoFixScope(t, store, "task-absent")
		requireCIAutoFixScopeUnchanged(t, before, after)
	})

	t.Run("explicitly disabled settings", func(t *testing.T) {
		store := newTestStore(t)
		disabled := false
		if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-disabled", "repo-a", 101,
			TaskPRAutomationOptionsPatch{AutoFixEnabled: &disabled}, false); err != nil {
			t.Fatalf("seed disabled options: %v", err)
		}
		before := snapshotCIAutoFixScope(t, store, "task-disabled")
		if err := report(store, "task-disabled", "session-current", "turn-current"); !errors.Is(err, ErrTaskCIAutoFixAttemptNotFound) {
			t.Fatalf("report error = %v, want %v", err, ErrTaskCIAutoFixAttemptNotFound)
		}
		after := snapshotCIAutoFixScope(t, store, "task-disabled")
		requireCIAutoFixScopeUnchanged(t, before, after)
	})

	t.Run("enabled without attempt", func(t *testing.T) {
		store := newTestStore(t)
		enabled := true
		if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-enabled", "repo-a", 101,
			TaskPRAutomationOptionsPatch{AutoFixEnabled: &enabled}, false); err != nil {
			t.Fatalf("seed enabled options: %v", err)
		}
		before := snapshotCIAutoFixScope(t, store, "task-enabled")
		if err := report(store, "task-enabled", "session-current", "turn-current"); !errors.Is(err, ErrTaskCIAutoFixAttemptNotFound) {
			t.Fatalf("report error = %v, want %v", err, ErrTaskCIAutoFixAttemptNotFound)
		}
		after := snapshotCIAutoFixScope(t, store, "task-enabled")
		requireCIAutoFixScopeUnchanged(t, before, after)
	})

	t.Run("foreign bound attempt across differently configured PRs", func(t *testing.T) {
		store := newTestStore(t)
		enabled := true
		disabled := false
		for _, target := range []struct {
			repositoryID string
			prNumber     int
			value        *bool
		}{
			{repositoryID: "repo-a", prNumber: 101, value: &enabled},
			{repositoryID: "repo-b", prNumber: 202, value: &disabled},
		} {
			if _, err := store.UpdateTaskPRAutomationOptions(ctx, "task-foreign", target.repositoryID, target.prNumber,
				TaskPRAutomationOptionsPatch{AutoFixEnabled: target.value}, false); err != nil {
				t.Fatalf("seed %s#%d options: %v", target.repositoryID, target.prNumber, err)
			}
		}
		if err := store.RecordTaskCIFixAttempt(ctx, TaskCIFixAttempt{
			TaskID: "task-foreign", RepositoryID: "repo-a", PRNumber: 101,
			Signature: "foreign-feedback", CheckpointJSON: `{}`,
			SessionID: "session-foreign", TurnID: "turn-foreign",
			ProviderGeneration: "head-foreign", State: TaskCIAutoFixAttemptRunning,
			IncrementRound: true, EnqueuedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("seed foreign attempt: %v", err)
		}
		before := snapshotCIAutoFixScope(t, store, "task-foreign")
		if err := report(store, "task-foreign", "session-current", "turn-current"); !errors.Is(err, ErrTaskCIAutoFixAttemptNotFound) {
			t.Fatalf("report error = %v, want %v", err, ErrTaskCIAutoFixAttemptNotFound)
		}
		after := snapshotCIAutoFixScope(t, store, "task-foreign")
		requireCIAutoFixScopeUnchanged(t, before, after)
	})

	t.Run("already reported attempt", func(t *testing.T) {
		store := newTestStore(t)
		attempt := TaskCIFixAttempt{
			TaskID: "task-reported", RepositoryID: "repo-a", PRNumber: 101,
			Signature: "reported-feedback", CheckpointJSON: `{}`,
			SessionID: "session-current", TurnID: "turn-current",
			ProviderGeneration: "head-reported", State: TaskCIAutoFixAttemptRunning,
			IncrementRound: true,
		}
		if err := store.RecordTaskCIFixAttempt(ctx, attempt); err != nil {
			t.Fatalf("seed reported attempt: %v", err)
		}
		if err := store.ReportTaskCIAutoFixOutcome(ctx, TaskCIAutoFixOutcomeReport{
			TaskID: attempt.TaskID, SessionID: attempt.SessionID, TurnID: attempt.TurnID,
			Outcome: TaskCIAutoFixOutcomeNonActionable, Summary: "first report",
		}); err != nil {
			t.Fatalf("record first outcome: %v", err)
		}
		before := snapshotCIAutoFixScope(t, store, attempt.TaskID)
		if err := store.ReportTaskCIAutoFixOutcome(ctx, TaskCIAutoFixOutcomeReport{
			TaskID: attempt.TaskID, SessionID: attempt.SessionID, TurnID: attempt.TurnID,
			Outcome: TaskCIAutoFixOutcomeActionTaken, Summary: "duplicate report",
		}); !errors.Is(err, ErrTaskCIAutoFixAttemptNotFound) {
			t.Fatalf("duplicate report error = %v, want %v", err, ErrTaskCIAutoFixAttemptNotFound)
		}
		after := snapshotCIAutoFixScope(t, store, attempt.TaskID)
		requireCIAutoFixScopeUnchanged(t, before, after)
	})
}

// @covers AC-UI-CI-PR-AUTOMATION-001.10
func TestStorePRAutoFixOutcomeAcceptsAllValidOutcomes(t *testing.T) {
	for _, test := range []struct {
		name          string
		outcome       TaskCIAutoFixOutcome
		wantState     TaskCIAutoFixAttemptState
		wantLastError bool
	}{
		{name: "action taken", outcome: TaskCIAutoFixOutcomeActionTaken, wantState: TaskCIAutoFixAttemptAwaitingProviderProgress},
		{name: "non actionable", outcome: TaskCIAutoFixOutcomeNonActionable, wantState: TaskCIAutoFixAttemptAcknowledged},
		{name: "blocked", outcome: TaskCIAutoFixOutcomeBlocked, wantState: TaskCIAutoFixAttemptAcknowledged, wantLastError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t)
			ctx := context.Background()
			attempt := TaskCIFixAttempt{
				TaskID: "task-valid-" + test.name, RepositoryID: "repo-valid", PRNumber: 303,
				Signature: "valid-feedback", CheckpointJSON: `{}`,
				SessionID: "session-valid", TurnID: "turn-valid",
				ProviderGeneration: "head-valid", State: TaskCIAutoFixAttemptRunning,
			}
			if err := store.RecordTaskCIFixAttempt(ctx, attempt); err != nil {
				t.Fatalf("seed attempt: %v", err)
			}
			if err := store.ReportTaskCIAutoFixOutcome(ctx, TaskCIAutoFixOutcomeReport{
				TaskID: attempt.TaskID, SessionID: attempt.SessionID, TurnID: attempt.TurnID,
				Outcome: test.outcome, Summary: "valid outcome",
			}); err != nil {
				t.Fatalf("record %s outcome: %v", test.outcome, err)
			}
			state, err := store.GetTaskCIPRState(ctx, attempt.TaskID, attempt.RepositoryID, attempt.PRNumber)
			if err != nil {
				t.Fatalf("get state: %v", err)
			}
			if state == nil {
				t.Fatal("attempt state is missing")
			}
			if state.AutoFixAttemptState != test.wantState || state.AutoFixAttemptOutcome != test.outcome {
				t.Fatalf("state = %+v, want state %q and outcome %q", state, test.wantState, test.outcome)
			}
			if (state.LastError != nil) != test.wantLastError {
				t.Fatalf("last error presence = %t, want %t", state.LastError != nil, test.wantLastError)
			}
		})
	}
}
