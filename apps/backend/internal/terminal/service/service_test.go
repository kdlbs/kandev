package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/terminal/models"
	"github.com/kandev/kandev/internal/terminal/repository"
)

// fakeBackend records calls and answers IsAlive from a map keyed by terminalID.
type fakeBackend struct {
	registered map[string]bool
	stopped    map[string]bool
	alive      map[string]bool
	envByID    map[string]string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		registered: map[string]bool{},
		stopped:    map[string]bool{},
		alive:      map[string]bool{},
		envByID:    map[string]string{},
	}
}

func (f *fakeBackend) Register(scopeID, terminalID string) {
	f.registered[terminalID] = true
	f.alive[terminalID] = true
	f.envByID[terminalID] = scopeID
}

func (f *fakeBackend) Stop(_ context.Context, _, terminalID string) error {
	f.stopped[terminalID] = true
	delete(f.alive, terminalID)
	return nil
}

func (f *fakeBackend) StopScope(_ context.Context, scopeID string) (int, error) {
	stopped := 0
	for terminalID, envID := range f.envByID {
		if envID != scopeID {
			continue
		}
		f.stopped[terminalID] = true
		delete(f.alive, terminalID)
		delete(f.envByID, terminalID)
		stopped++
	}
	return stopped, nil
}

func (f *fakeBackend) IsAlive(_, terminalID string) bool {
	return f.alive[terminalID]
}

// fakeTaskEnvReader implements the minimal taskEnvironmentReader interface
// for testing CleanupTask when no ordinary terminal rows exist.
type fakeTaskEnvReader struct {
	env *taskmodels.TaskEnvironment
}

func (f *fakeTaskEnvReader) GetTaskEnvironmentByTaskID(_ context.Context, _ string) (*taskmodels.TaskEnvironment, error) {
	return f.env, nil
}

func setupService(t *testing.T) (*Service, *fakeBackend) {
	t.Helper()
	// shared-cache + MaxOpenConns(1) mirror the repository test setup so
	// every connection in the sqlx pool talks to the same in-memory DB.
	rawDB, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := repository.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	be := newFakeBackend()
	return New(repo, be, nil), be
}

func TestCreate_RegistersWithBackend(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	term, err := svc.Create(ctx, "task-1", "env-1", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if term.Seq != 1 {
		t.Errorf("seq = %d, want 1", term.Seq)
	}
	if !be.registered[term.ID] {
		t.Errorf("backend was not asked to register %s", term.ID)
	}
}

func TestList_BlendsDBAndPTYStatus(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	t1, _ := svc.Create(ctx, "task-1", "env-1", "")
	t2, _ := svc.Create(ctx, "task-1", "env-1", "")
	delete(be.alive, t2.ID) // simulate dead PTY

	items, err := svc.List(ctx, "task-1", true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	if items[0].ID != t1.ID || items[0].PTYStatus != PTYStatusRunning {
		t.Errorf("item 0 = %+v, want running %s", items[0], t1.ID)
	}
	if items[1].ID != t2.ID || items[1].PTYStatus != PTYStatusStopped {
		t.Errorf("item 1 = %+v, want stopped %s", items[1], t2.ID)
	}
}

func TestList_FilterParked(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, "task-1", "env-1", "")
	t2, _ := svc.Create(ctx, "task-1", "env-1", "")
	_ = svc.Park(ctx, "task-1", t2.ID)

	open, _ := svc.List(ctx, "task-1", false)
	if len(open) != 1 {
		t.Errorf("open count = %d, want 1", len(open))
	}
	all, _ := svc.List(ctx, "task-1", true)
	if len(all) != 2 {
		t.Errorf("all count = %d, want 2", len(all))
	}
}

func TestRename_UpdatesDisplayName(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	term, _ := svc.Create(ctx, "task-1", "env-1", "")
	name := "build watcher"
	if err := svc.Rename(ctx, "task-1", term.ID, &name); err != nil {
		t.Fatalf("rename: %v", err)
	}

	items, _ := svc.List(ctx, "task-1", true)
	if items[0].DisplayName != "build watcher" {
		t.Errorf("display = %q, want build watcher", items[0].DisplayName)
	}
}

func TestPark_DoesNotStopPTY(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	term, _ := svc.Create(ctx, "task-1", "env-1", "")
	if err := svc.Park(ctx, "task-1", term.ID); err != nil {
		t.Fatalf("park: %v", err)
	}
	if be.stopped[term.ID] {
		t.Errorf("park stopped PTY; should leave running")
	}
	if !be.IsAlive("env-1", term.ID) {
		t.Errorf("PTY no longer alive after park")
	}
}

func TestResume_SetsStateOpen(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	term, _ := svc.Create(ctx, "task-1", "env-1", "")
	_ = svc.Park(ctx, "task-1", term.ID)
	if err := svc.Resume(ctx, "task-1", term.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}

	items, _ := svc.List(ctx, "task-1", false)
	if len(items) != 1 || items[0].State != string(models.StateOpen) {
		t.Errorf("resume: items = %+v", items)
	}
}

func TestDestroy_StopsAndDeletes(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	term, _ := svc.Create(ctx, "task-1", "env-1", "")
	if err := svc.Destroy(ctx, "task-1", term.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !be.stopped[term.ID] {
		t.Errorf("PTY not stopped")
	}
	items, _ := svc.List(ctx, "task-1", true)
	if len(items) != 0 {
		t.Errorf("rows remain after destroy: %d", len(items))
	}
}

func TestCleanupTask_StopsAllAndDeletes(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	t1, _ := svc.Create(ctx, "task-1", "env-1", "")
	t2, _ := svc.Create(ctx, "task-1", "env-1", "")
	other, _ := svc.Create(ctx, "task-2", "env-2", "")
	be.Register("env-1", "bottom-panel")

	n, err := svc.CleanupTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if n != 2 {
		t.Errorf("cleanup count = %d, want 2", n)
	}
	if !be.stopped[t1.ID] || !be.stopped[t2.ID] {
		t.Errorf("not all stopped: %+v", be.stopped)
	}
	if !be.stopped["bottom-panel"] {
		t.Errorf("unmanaged shell in task environment was not stopped")
	}
	if be.stopped[other.ID] {
		t.Errorf("other task affected: %s", other.ID)
	}
}

// TestCleanupTask_StopsShellsWhenNoOrdinaryTerminals ensures that bottom-panel
// and script shells are still torn down when a task has no persisted ordinary
// terminal rows (the env ID can only be discovered via the task layer).
func TestCleanupTask_StopsShellsWhenNoOrdinaryTerminals(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	// No ordinary terminals for task-1, but there is a task environment.
	be.Register("env-orphan", "bottom-panel")
	svc.SetTaskEnvironmentReader(&fakeTaskEnvReader{
		env: &taskmodels.TaskEnvironment{ID: "env-orphan", TaskID: "task-1"},
	})

	_, err := svc.CleanupTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !be.stopped["bottom-panel"] {
		t.Errorf("unmanaged shell was not stopped when no ordinary terminals exist")
	}
}

func TestGuard_RejectsBottomPanel(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	name := "x"
	if err := svc.Rename(ctx, "task-1", "bottom-panel", &name); err == nil {
		t.Error("expected guard error for bottom-panel rename")
	}
	if err := svc.Park(ctx, "task-1", "bottom-panel"); err == nil {
		t.Error("expected guard error for bottom-panel park")
	}
	if err := svc.Destroy(ctx, "task-1", "bottom-panel"); err == nil {
		t.Error("expected guard error for bottom-panel destroy")
	}
}

// TestRename_RejectsCrossTask ensures a caller supplying the wrong task_id
// cannot rename a terminal owned by another task. Same shape applies to
// park/resume/destroy via the shared requireOwnership helper.
func TestRename_RejectsCrossTask(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, "task-a", "env-a", "")
	name := "stolen"
	err := svc.Rename(ctx, "task-b", owned.ID, &name)
	if err == nil {
		t.Fatal("expected cross-task rename to be rejected")
	}
	if !errors.Is(err, ErrTaskMismatch) {
		t.Errorf("expected ErrTaskMismatch, got %v", err)
	}
}

// TestPark_RejectsCrossTask same defense for park.
func TestPark_RejectsCrossTask(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, "task-a", "env-a", "")
	err := svc.Park(ctx, "task-b", owned.ID)
	if !errors.Is(err, ErrTaskMismatch) {
		t.Errorf("expected ErrTaskMismatch, got %v", err)
	}
}

// TestDestroy_RejectsCrossTask same defense for destroy.
func TestDestroy_RejectsCrossTask(t *testing.T) {
	svc, be := setupService(t)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, "task-a", "env-a", "")
	err := svc.Destroy(ctx, "task-b", owned.ID)
	if !errors.Is(err, ErrTaskMismatch) {
		t.Errorf("expected ErrTaskMismatch, got %v", err)
	}
	if be.stopped[owned.ID] {
		t.Error("destroy should not have torn down the PTY across tasks")
	}
}

// TestRename_EmptyTaskIDRejected ensures the previous "empty taskID
// skips" carve-out is gone — an unauthenticated client cannot mutate any
// terminal by raw id just by omitting task_id.
func TestRename_EmptyTaskIDRejected(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, "task-a", "env-a", "")
	name := "stolen"
	err := svc.Rename(ctx, "", owned.ID, &name)
	if !errors.Is(err, ErrTaskMismatch) {
		t.Errorf("empty taskID should reject with ErrTaskMismatch, got %v", err)
	}
}

func TestGuard_RejectsScriptPrefix(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	name := "x"
	if err := svc.Rename(ctx, "task-1", "script-abc", &name); err == nil {
		t.Error("expected guard error for script- rename")
	}
}

func TestIsManaged(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"some-uuid", true},
		{"bottom-panel", false},
		{"script-anything", false},
		{"shell-uuid", true},
	}
	for _, c := range cases {
		if got := IsManaged(c.id); got != c.want {
			t.Errorf("IsManaged(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}
