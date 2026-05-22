package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/terminal/models"
	"github.com/kandev/kandev/internal/terminal/repository"
)

// fakeBackend records calls and answers IsAlive from a map keyed by terminalID.
type fakeBackend struct {
	registered map[string]bool
	stopped    map[string]bool
	alive      map[string]bool
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		registered: map[string]bool{},
		stopped:    map[string]bool{},
		alive:      map[string]bool{},
	}
}

func (f *fakeBackend) Register(_, terminalID string) {
	f.registered[terminalID] = true
	f.alive[terminalID] = true
}

func (f *fakeBackend) Stop(_ context.Context, _, terminalID string) error {
	f.stopped[terminalID] = true
	delete(f.alive, terminalID)
	return nil
}

func (f *fakeBackend) IsAlive(_, terminalID string) bool {
	return f.alive[terminalID]
}

func setupService(t *testing.T) (*Service, *fakeBackend) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
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
	_ = svc.Park(ctx, t2.ID)

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
	if err := svc.Rename(ctx, term.ID, &name); err != nil {
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
	if err := svc.Park(ctx, term.ID); err != nil {
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
	_ = svc.Park(ctx, term.ID)
	if err := svc.Resume(ctx, term.ID); err != nil {
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
	if err := svc.Destroy(ctx, term.ID); err != nil {
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
	if be.stopped[other.ID] {
		t.Errorf("other task affected: %s", other.ID)
	}
}

func TestGuard_RejectsBottomPanel(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	name := "x"
	if err := svc.Rename(ctx, "bottom-panel", &name); err == nil {
		t.Error("expected guard error for bottom-panel rename")
	}
	if err := svc.Park(ctx, "bottom-panel"); err == nil {
		t.Error("expected guard error for bottom-panel park")
	}
	if err := svc.Destroy(ctx, "bottom-panel"); err == nil {
		t.Error("expected guard error for bottom-panel destroy")
	}
}

func TestGuard_RejectsScriptPrefix(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	name := "x"
	if err := svc.Rename(ctx, "script-abc", &name); err == nil {
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
