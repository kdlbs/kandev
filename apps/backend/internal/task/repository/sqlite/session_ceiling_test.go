package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func ceilingDeferredMetadata() map[string]interface{} {
	return map[string]interface{}{
		models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.CeilingDeferredKey:   true,
			models.CeilingLaunchKindKey: "start_task",
		},
	}
}

func archiveTaskForCeilingTests(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET archived_at = ? WHERE id = ?`), time.Now().UTC(), taskID); err != nil {
		t.Fatalf("archive %s: %v", taskID, err)
	}
}

// TestAdmittedSessionIDsMatchSQLFilter cross-checks ListAdmittedSessionIDs's SQL
// state filter against models.IsAdmittedSessionState for every state in
// models.AllTaskSessionStates, the package's single canonical state list. Editing
// the SQL without updating the predicate (or vice versa) fails here instead of
// only drifting silently — but only for states already present in
// AllTaskSessionStates: Go does not enforce switch/slice exhaustiveness, so a new
// TaskSessionState constant added to models.go without also being added to
// AllTaskSessionStates is not caught by this test (or by any linter — the
// exhaustive linter is not enabled in this repo). AC-1, AC-3, AC-3a.
func TestAdmittedSessionIDsMatchSQLFilter(t *testing.T) {
	for _, state := range models.AllTaskSessionStates {
		t.Run(string(state), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := "task-admitted-" + string(state)
			sessionID := "session-admitted-" + string(state)

			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Admitted state check"}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID:     sessionID,
				TaskID: taskID,
				State:  state,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}

			got, err := repo.ListAdmittedSessionIDs(ctx)
			if err != nil {
				t.Fatalf("ListAdmittedSessionIDs: %v", err)
			}

			want := models.IsAdmittedSessionState(state)
			found := false
			for _, id := range got {
				if id == sessionID {
					found = true
				}
			}
			if found != want {
				t.Fatalf("ListAdmittedSessionIDs returned %v for state %q; found=%v want=%v", got, state, found, want)
			}
		})
	}
}

// TestListAdmittedSessionIDsIgnoresTaskShape pins AC-2 and AC-2a: a session counts
// regardless of whether its task is archived, ephemeral, or automation-origin, and
// regardless of config-mode or passthrough on the session itself.
func TestListAdmittedSessionIDsIgnoresTaskShape(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		task    *models.Task
		session *models.TaskSession
		archive bool
	}{
		{
			name:    "archived task",
			task:    &models.Task{ID: "task-ceil-archived", Title: "Archived"},
			session: &models.TaskSession{ID: "session-ceil-archived", TaskID: "task-ceil-archived", State: models.TaskSessionStateRunning},
			archive: true,
		},
		{
			name:    "ephemeral task",
			task:    &models.Task{ID: "task-ceil-ephemeral", Title: "Ephemeral", IsEphemeral: true},
			session: &models.TaskSession{ID: "session-ceil-ephemeral", TaskID: "task-ceil-ephemeral", State: models.TaskSessionStateStarting},
		},
		{
			name:    "automation origin task",
			task:    &models.Task{ID: "task-ceil-automation", Title: "Automation", Origin: models.TaskOriginAutomationRun},
			session: &models.TaskSession{ID: "session-ceil-automation", TaskID: "task-ceil-automation", State: models.TaskSessionStateRunning},
		},
		{
			name: "config mode session",
			task: &models.Task{ID: "task-ceil-config", Title: "Config"},
			session: &models.TaskSession{
				ID: "session-ceil-config", TaskID: "task-ceil-config", State: models.TaskSessionStateRunning,
				Metadata: map[string]interface{}{"config_mode": true},
			},
		},
		{
			name:    "passthrough session",
			task:    &models.Task{ID: "task-ceil-passthrough", Title: "Passthrough"},
			session: &models.TaskSession{ID: "session-ceil-passthrough", TaskID: "task-ceil-passthrough", State: models.TaskSessionStateStarting, IsPassthrough: true},
		},
	}

	for _, tc := range cases {
		if err := repo.CreateTask(ctx, tc.task); err != nil {
			t.Fatalf("CreateTask(%s): %v", tc.name, err)
		}
		if err := repo.CreateTaskSession(ctx, tc.session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", tc.name, err)
		}
		if tc.archive {
			archiveTaskForCeilingTests(t, repo, tc.task.ID)
		}
	}

	got, err := repo.ListAdmittedSessionIDs(ctx)
	if err != nil {
		t.Fatalf("ListAdmittedSessionIDs: %v", err)
	}
	index := make(map[string]bool, len(got))
	for _, id := range got {
		index[id] = true
	}
	for _, tc := range cases {
		if !index[tc.session.ID] {
			t.Errorf("ListAdmittedSessionIDs omitted %s (%s); got %v", tc.session.ID, tc.name, got)
		}
	}
	if len(got) != len(cases) {
		t.Fatalf("ListAdmittedSessionIDs returned %d ids, want %d: %v", len(got), len(cases), got)
	}
}

// TestListTasksWithCeilingDeferralAppliesNoneOfTheThreeFilters pins AC-50b: the
// new method must not inherit ListTasksWithMetadataKey's archived_at, is_ephemeral
// or automation-origin filters. Each one would strand records the sweep is
// required to find.
func TestListTasksWithCeilingDeferralAppliesNoneOfTheThreeFilters(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	tasks := []*models.Task{
		{ID: "task-defer-archived", Title: "Archived", Metadata: ceilingDeferredMetadata()},
		{ID: "task-defer-ephemeral", Title: "Ephemeral", IsEphemeral: true, Metadata: ceilingDeferredMetadata()},
		{ID: "task-defer-automation", Title: "Automation", Origin: models.TaskOriginAutomationRun, Metadata: ceilingDeferredMetadata()},
		{ID: "task-defer-plain", Title: "Plain", Metadata: ceilingDeferredMetadata()},
	}
	for _, task := range tasks {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}
	archiveTaskForCeilingTests(t, repo, "task-defer-archived")

	got, err := repo.ListTasksWithCeilingDeferral(ctx)
	if err != nil {
		t.Fatalf("ListTasksWithCeilingDeferral: %v", err)
	}
	index := make(map[string]bool, len(got))
	for _, task := range got {
		index[task.ID] = true
	}
	for _, task := range tasks {
		if !index[task.ID] {
			t.Errorf("ListTasksWithCeilingDeferral omitted %s; got %d tasks", task.ID, len(got))
		}
	}
	if len(got) != len(tasks) {
		t.Fatalf("ListTasksWithCeilingDeferral returned %d tasks, want %d", len(got), len(tasks))
	}
}

// TestListTasksWithCeilingDeferralOrdersByIDAscending pins AC-50c: ordering is by
// tasks.id ascending alone. Insertion order below is deliberately not id order,
// and updated_at is deliberately moved afterwards on the id-first task so a
// three-column sort copied from ListTasksWithMetadataKey would fail here.
func TestListTasksWithCeilingDeferralOrdersByIDAscending(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	for _, id := range []string{"ceil-c", "ceil-a", "ceil-b"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: id, Metadata: ceilingDeferredMetadata()}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	// Push the id-first task to the newest updated_at. Under t.id ASC it stays
	// first; under a leading t.updated_at ASC term it would sort last.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET updated_at = ? WHERE id = ?`), time.Now().UTC().Add(time.Hour), "ceil-a"); err != nil {
		t.Fatalf("bump updated_at: %v", err)
	}

	got, err := repo.ListTasksWithCeilingDeferral(ctx)
	if err != nil {
		t.Fatalf("ListTasksWithCeilingDeferral: %v", err)
	}
	var ids []string
	for _, task := range got {
		ids = append(ids, task.ID)
	}
	want := []string{"ceil-a", "ceil-b", "ceil-c"}
	if len(ids) != len(want) {
		t.Fatalf("ListTasksWithCeilingDeferral returned %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ListTasksWithCeilingDeferral returned %v, want %v", ids, want)
		}
	}
}

// TestListTasksWithCeilingDeferralSkipsOtherDeferredLaunchRecords pins the
// discriminator: deferred_launch is a shared record carrying three independent
// meanings, and only the ceiling_deferred one is this sweep's candidate.
func TestListTasksWithCeilingDeferralSkipsOtherDeferredLaunchRecords(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	tasks := []*models.Task{
		{ID: "skip-no-metadata", Title: "None"},
		{ID: "skip-empty-record", Title: "Empty", Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{},
		}},
		{ID: "skip-start-when-unblocked", Title: "Unblocked", Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		}},
		{ID: "skip-ceiling-false", Title: "False", Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				models.CeilingDeferredKey: false,
			},
		}},
		{ID: "keep-ceiling-true", Title: "True", Metadata: ceilingDeferredMetadata()},
	}
	for _, task := range tasks {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}

	got, err := repo.ListTasksWithCeilingDeferral(ctx)
	if err != nil {
		t.Fatalf("ListTasksWithCeilingDeferral: %v", err)
	}
	if len(got) != 1 || got[0].ID != "keep-ceiling-true" {
		var ids []string
		for _, task := range got {
			ids = append(ids, task.ID)
		}
		t.Fatalf("ListTasksWithCeilingDeferral returned %v, want [keep-ceiling-true]", ids)
	}
	if !models.HasCeilingDeferredIntent(got[0]) {
		t.Fatalf("scanned task lost its ceiling deferral: %+v", got[0].Metadata)
	}
}
