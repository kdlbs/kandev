//go:build !windows

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestServiceInitializeLocalRepositoryRejectsForeignOwnedStickyParent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing directory ownership requires root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, os.ModeSticky|0o777); err != nil {
		t.Fatalf("Chmod parent: %v", err)
	}
	if err := os.Chown(parent, 12345, 12345); err != nil {
		t.Fatalf("Chown parent: %v", err)
	}
	assertLocalRepositoryParentRejected(t, parent)
}

func TestServiceInitializeLocalRepositoryRejectsForeignOwnedAncestor(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing directory ownership requires root")
	}
	ancestor := filepath.Join(t.TempDir(), "foreign")
	parent := filepath.Join(ancestor, "projects")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("MkdirAll parent: %v", err)
	}
	if err := os.Chown(ancestor, 12345, 12345); err != nil {
		t.Fatalf("Chown ancestor: %v", err)
	}
	assertLocalRepositoryParentRejected(t, parent)
}

func assertLocalRepositoryParentRejected(t *testing.T, parent string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1", Name: "new-project", ParentPath: parent,
	})
	if !errors.Is(err, ErrInvalidLocalRepositoryInitialization) {
		t.Fatalf("error = %v, want ErrInvalidLocalRepositoryInitialization", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "new-project")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target stat error = %v, want not exist", statErr)
	}
	assertNoInitializedRepositories(t, repo, "ws-1")
}
