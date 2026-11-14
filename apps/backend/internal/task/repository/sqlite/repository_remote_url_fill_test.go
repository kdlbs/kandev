package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFillRepositoryRemoteURLOnlyFillsAnEmptyURL(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'ws-1', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	for _, row := range []*models.Repository{
		{ID: "empty", WorkspaceID: "ws-1", Name: "empty", SourceType: "provider"},
		{ID: "set", WorkspaceID: "ws-1", Name: "set", SourceType: "provider", RemoteURL: "https://example.test/own.git"},
	} {
		if err := repo.CreateRepository(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.ID, err)
		}
	}
	// A concurrent settings save after the caller's read must survive.
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE repositories SET setup_script = ? WHERE id = ?`), "make setup", "empty"); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}

	filled, err := repo.FillRepositoryRemoteURL(ctx, "empty", "https://example.test/source.git")
	if err != nil || !filled {
		t.Fatalf("fill empty = %v, %v", filled, err)
	}
	filled, err = repo.FillRepositoryRemoteURL(ctx, "set", "https://example.test/source.git")
	if err != nil || filled {
		t.Fatalf("fill set = %v, %v", filled, err)
	}

	empty, err := repo.GetRepository(ctx, "empty")
	if err != nil || empty.RemoteURL != "https://example.test/source.git" || empty.SetupScript != "make setup" {
		t.Fatalf("empty after fill = %+v, %v", empty, err)
	}
	set, err := repo.GetRepository(ctx, "set")
	if err != nil || set.RemoteURL != "https://example.test/own.git" {
		t.Fatalf("set after fill = %+v, %v", set, err)
	}
}
