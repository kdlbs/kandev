package pause_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/pause"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newIntegrationService wires pause.Service to a real, freshly-migrated
// office SQLite repository. Unlike service_test.go's fakeRepo suite (which
// scripts exact race sequencing), this proves the interfaces this package
// declares are actually satisfied by the concrete repository main.go wires
// in, and that a real Pause/Resume round trip persists correctly.
func newIntegrationService(t *testing.T) *pause.Service {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	// tasks is owned by internal/task/repository/sqlite, not this
	// package's initSchema; a minimal stand-in lets
	// ListLiveRoutineTaskIDsForWorkspace's join resolve cleanly instead of
	// failing the sweep on a table that simply isn't part of this fixture.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY, state TEXT NOT NULL DEFAULT 'TODO'
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return pause.NewService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}}, logger.Default())
}

// TestIntegration_PauseThenResumeRoundTrip proves the real repository
// satisfies pause.Repository and a full Pause -> PauseState -> Resume ->
// PauseState cycle behaves as the design specifies, end to end.
func TestIntegration_PauseThenResumeRoundTrip(t *testing.T) {
	svc := newIntegrationService(t)
	ctx := context.Background()

	result, err := svc.Pause(ctx, "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Pause == nil || result.Pause.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected pause record: %+v", result.Pause)
	}

	active, err := svc.PauseState(ctx, "ws-1")
	if err != nil {
		t.Fatalf("PauseState: %v", err)
	}
	if active == nil {
		t.Fatal("expected an active pause after Pause")
	}

	resumeResult, err := svc.Resume(ctx, "ws-1", "resolved", "user-1", "user")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !resumeResult.Released {
		t.Fatal("expected Released=true")
	}

	active, err = svc.PauseState(ctx, "ws-1")
	if err != nil {
		t.Fatalf("PauseState after resume: %v", err)
	}
	if active != nil {
		t.Fatalf("expected no active pause after resume, got %+v", active)
	}
}
