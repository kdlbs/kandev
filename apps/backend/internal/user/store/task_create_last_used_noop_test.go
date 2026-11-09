package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/user/models"
)

func newTaskCreateLastUsedRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func TestTaskCreateLastUsedNoOpPreservesRevisionAndTimestamp(t *testing.T) {
	ctx := context.Background()
	repo := newTaskCreateLastUsedRepo(t)
	initial := models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "main",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		WorkflowIDsByWorkspace: map[string]string{
			"workspace-1": "workflow-1",
		},
	}
	if _, changed, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, initial); err != nil || !changed {
		t.Fatalf("seed update: changed=%v err=%v", changed, err)
	}
	before, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}

	got, changed, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, initial)
	if err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if changed {
		t.Fatal("identical choices must not report a change")
	}
	if got.Revision != before.Revision {
		t.Fatalf("revision changed on no-op: before=%d after=%d", before.Revision, got.Revision)
	}
	if !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("timestamp changed on no-op: before=%s after=%s", before.UpdatedAt, got.UpdatedAt)
	}
	if got.TaskCreateLastUsed.RepositoryID != initial.RepositoryID ||
		got.TaskCreateLastUsed.Branch != initial.Branch {
		t.Fatalf("no-op returned different choices: %+v", got.TaskCreateLastUsed)
	}
}

func TestTaskCreateLastUsedChangedChoicesPreserveOmittedFields(t *testing.T) {
	ctx := context.Background()
	repo := newTaskCreateLastUsedRepo(t)
	if _, _, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "main",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		WorkflowIDsByWorkspace: map[string]string{
			"workspace-1": "workflow-1",
			"workspace-2": "workflow-2",
		},
	}); err != nil {
		t.Fatalf("seed update: %v", err)
	}

	got, changed, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		Branch: "feature",
		WorkflowIDsByWorkspace: map[string]string{
			"workspace-2": "workflow-new",
		},
	})
	if err != nil {
		t.Fatalf("changed update: %v", err)
	}
	if !changed {
		t.Fatal("changed choices must report a change")
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-1" ||
		got.TaskCreateLastUsed.AgentProfileID != "agent-1" ||
		got.TaskCreateLastUsed.ExecutorProfileID != "exec-1" {
		t.Fatalf("omitted fields were not preserved: %+v", got.TaskCreateLastUsed)
	}
	if got.TaskCreateLastUsed.Branch != "feature" {
		t.Fatalf("branch = %q, want feature", got.TaskCreateLastUsed.Branch)
	}
	if got.TaskCreateLastUsed.WorkflowIDsByWorkspace["workspace-1"] != "workflow-1" ||
		got.TaskCreateLastUsed.WorkflowIDsByWorkspace["workspace-2"] != "workflow-new" {
		t.Fatalf("workspace history = %+v", got.TaskCreateLastUsed.WorkflowIDsByWorkspace)
	}
}

func TestTaskCreateLastUsedMissingUserReturnsError(t *testing.T) {
	repo := newTaskCreateLastUsedRepo(t)
	settings, changed, err := repo.UpdateTaskCreateLastUsed(context.Background(), "missing-user", models.TaskCreateLastUsed{
		Branch: "feature",
	})
	if changed {
		t.Fatal("missing user must not report a change")
	}
	if err == nil || settings != nil {
		t.Fatalf("missing user: settings=%+v err=%v", settings, err)
	}
}

func TestTaskCreateLastUsedConcurrentColorSurvives(t *testing.T) {
	ctx := context.Background()
	repo := newTaskCreateLastUsedRepo(t)
	initial, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	if err := repo.upsertUserSettingsForTest(ctx, initial, initial.Revision); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	writeColor := func(color string) error {
		for attempt := 0; attempt < 5; attempt++ {
			current, getErr := repo.GetUserSettings(ctx, DefaultUserID)
			if getErr != nil {
				return getErr
			}
			next := *current
			next.SidebarTaskColors = models.CloneSidebarTaskColors(current.SidebarTaskColors)
			if next.SidebarTaskColors == nil {
				next.SidebarTaskColors = map[string]*string{}
			}
			next.SidebarTaskColors["task-color"] = &color
			_, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, &next, nil, current.Revision)
			if errors.Is(err, ErrUserSettingsRevisionConflict) {
				time.Sleep(time.Millisecond)
				continue
			}
			return err
		}
		return ErrUserSettingsRevisionConflict
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- writeColor("blue")
	}()
	go func() {
		defer wg.Done()
		_, _, updateErr := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
			RepositoryID: "repo-concurrent",
			Branch:       "main",
		})
		errs <- updateErr
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write: %v", err)
		}
	}

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if color := got.SidebarTaskColors["task-color"]; color == nil || *color != "blue" {
		t.Fatalf("concurrent color was lost: %+v", got.SidebarTaskColors)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-concurrent" {
		t.Fatalf("last-used choice was lost: %+v", got.TaskCreateLastUsed)
	}
}

func (r *sqliteRepository) upsertUserSettingsForTest(ctx context.Context, settings *models.UserSettings, revision int64) error {
	_, err := r.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, nil, revision)
	return err
}
