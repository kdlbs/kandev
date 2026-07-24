package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileWorkspaceSources_RejectsMissingFolderTarget(t *testing.T) {
	err := reconcileWorkspaceSources(context.Background(), t.TempDir(), []WorkspaceFolderSpec{{Name: "missing", LocalPath: "/definitely/not/a/kandev-folder"}})
	if err == nil {
		t.Fatal("missing durable folder target was accepted")
	}
}

func TestReconcileWorkspaceRepositories_RecreatesMissingOwnedLink(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if got, err := os.Readlink(filepath.Join(root, "api")); err != nil || got != source {
		t.Fatalf("repository link = %q, %v; want %q", got, err, source)
	}
	if err := os.Remove(filepath.Join(root, "api")); err != nil {
		t.Fatal(err)
	}
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcile after reset: %v", err)
	}
}
