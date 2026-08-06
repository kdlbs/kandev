package quickterminal

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

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
