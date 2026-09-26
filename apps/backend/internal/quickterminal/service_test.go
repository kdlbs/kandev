package quickterminal

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/loginpty"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/quickterminal/models"
	"github.com/kandev/kandev/internal/quickterminal/repository"
	"github.com/kandev/kandev/internal/user/store"
)

func serviceRepository(t *testing.T) *repository.Repository {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := repository.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return repo
}

func serviceManager(t *testing.T) *loginpty.Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return loginpty.NewManager(log, nil)
}

func TestListMarksAConnectingDescriptorWithoutPTYAsUnavailable(t *testing.T) {
	repo := serviceRepository(t)
	ctx := context.Background()
	tabID := "11111111-1111-4111-8111-111111111111"
	if _, err := repo.Create(ctx, store.DefaultUserID, "workspace-1", tabID); err != nil {
		t.Fatalf("create: %v", err)
	}

	svc := NewService(repo, nil, nil)
	tabs, err := svc.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tabs) != 1 || tabs[0].Status != "exited" || tabs[0].Error == "" {
		t.Fatalf("tabs = %#v, want unavailable exited descriptor", tabs)
	}
	if tabs[0].SessionID != nil {
		t.Fatalf("session id = %v, want nil", tabs[0].SessionID)
	}
}

func TestCreateRejectsNonUUIDAndRequiresWorkspace(t *testing.T) {
	svc := NewService(serviceRepository(t), nil, nil)
	if _, err := svc.Create(context.Background(), "workspace-1", "not-a-uuid"); err != ErrInvalidTabID {
		t.Fatalf("invalid tab id error = %v, want %v", err, ErrInvalidTabID)
	}
	if _, err := svc.Create(context.Background(), "", "11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("expected empty workspace to be rejected")
	}
}

func TestCreateRejectsReusingATabIDInAnotherWorkspace(t *testing.T) {
	repo := serviceRepository(t)
	svc := NewService(repo, nil, nil)
	ctx := context.Background()
	tabID := "11111111-1111-4111-8111-111111111111"

	if _, err := svc.Create(ctx, "workspace-1", tabID); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := svc.Create(ctx, "workspace-2", tabID); err != repository.ErrTabIDConflict {
		t.Fatalf("cross-workspace reuse error = %v, want %v", err, repository.ErrTabIDConflict)
	}
}

func TestListKeepsARunningQuickTerminalSessionAttached(t *testing.T) {
	repo := serviceRepository(t)
	mgr := serviceManager(t)
	ctx := context.Background()
	tabID := "11111111-1111-4111-8111-111111111111"

	tab, err := repo.Create(ctx, store.DefaultUserID, "workspace-1", tabID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sess, err := mgr.StartWithKey(loginpty.HostShellAgentID+":"+tabID, loginpty.HostShellAgentID, []string{"sh", "-c", "sleep 1"}, 80, 24)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopAll() })
	if err := repo.UpdateLifecycle(ctx, tab.UserID, tab.TabID, sess.ID, models.StatusRunning, nil, ""); err != nil {
		t.Fatalf("UpdateLifecycle: %v", err)
	}

	svc := NewService(repo, mgr, nil)
	tabs, err := svc.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tabs) != 1 {
		t.Fatalf("tabs count = %d, want 1", len(tabs))
	}
	if tabs[0].SessionID == nil || *tabs[0].SessionID != sess.ID {
		t.Fatalf("session id = %v, want %s", tabs[0].SessionID, sess.ID)
	}
	if tabs[0].Status != models.StatusRunning {
		t.Fatalf("status = %q, want %q", tabs[0].Status, models.StatusRunning)
	}
}

func TestListMarksMissingQuickTerminalSessionUnavailable(t *testing.T) {
	repo := serviceRepository(t)
	ctx := context.Background()
	tabID := "11111111-1111-4111-8111-111111111111"

	tab, err := repo.Create(ctx, store.DefaultUserID, "workspace-1", tabID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateLifecycle(ctx, tab.UserID, tab.TabID, "missing-session", models.StatusRunning, nil, ""); err != nil {
		t.Fatalf("UpdateLifecycle: %v", err)
	}

	svc := NewService(repo, serviceManager(t), nil)
	tabs, err := svc.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tabs) != 1 {
		t.Fatalf("tabs count = %d, want 1", len(tabs))
	}
	if tabs[0].SessionID != nil {
		t.Fatalf("session id = %v, want nil", tabs[0].SessionID)
	}
	if tabs[0].Status != models.StatusExited || tabs[0].Error == "" {
		t.Fatalf("tab = %#v, want exited/unavailable", tabs[0])
	}
}

func TestBindHostShellSessionPersistsRunningState(t *testing.T) {
	repo := serviceRepository(t)
	mgr := serviceManager(t)
	ctx := context.Background()
	tabID := "11111111-1111-4111-8111-111111111111"

	tab, err := repo.Create(ctx, store.DefaultUserID, "workspace-1", tabID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sess, err := mgr.StartWithKey(loginpty.HostShellAgentID+":"+tabID, loginpty.HostShellAgentID, []string{"sh", "-c", "sleep 1"}, 80, 24)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopAll() })

	svc := NewService(repo, mgr, nil)
	if err := svc.BindHostShellSession(ctx, tab.TabID, sess.ID); err != nil {
		t.Fatalf("BindHostShellSession: %v", err)
	}
	stored, err := repo.Get(ctx, tab.UserID, tab.TabID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.SessionID == nil || *stored.SessionID != sess.ID {
		t.Fatalf("session id = %v, want %s", stored.SessionID, sess.ID)
	}
	if stored.Status != models.StatusRunning {
		t.Fatalf("status = %q, want %q", stored.Status, models.StatusRunning)
	}
}
