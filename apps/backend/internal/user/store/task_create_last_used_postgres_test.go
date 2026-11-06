package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/user/models"
)

func newTaskCreateLastUsedPostgresRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	// OpenIsolatedPostgres caps the pool at one connection. The concurrent
	// test needs independent connections so the two writers can overlap.
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}
	return repo
}

func TestPostgresTaskCreateLastUsedNoOpPreservesRevision(t *testing.T) {
	ctx := context.Background()
	repo := newTaskCreateLastUsedPostgresRepo(t)
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
	if got.Revision != before.Revision || !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("no-op changed revision/timestamp: before=%d/%s after=%d/%s",
			before.Revision, before.UpdatedAt, got.Revision, got.UpdatedAt)
	}
}

func TestPostgresTaskCreateLastUsedConcurrentColorSurvives(t *testing.T) {
	ctx := context.Background()
	repo := newTaskCreateLastUsedPostgresRepo(t)
	initial, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, initial, nil, initial.Revision); err != nil {
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
			t.Fatalf("concurrent postgres write: %v", err)
		}
	}

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if color := got.SidebarTaskColors["task-color"]; color == nil || *color != "blue" {
		t.Fatalf("concurrent postgres color was lost: %+v", got.SidebarTaskColors)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-concurrent" {
		t.Fatalf("postgres last-used choice was lost: %+v", got.TaskCreateLastUsed)
	}
}
