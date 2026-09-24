package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveManifestRejectsForeignRepositoryPath(t *testing.T) {
	foreignRepo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(foreignRepo, "foreign-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "recorded-worktree", TaskID: "recorded-task", RepositoryID: "recorded-repository",
		Path: foreignRepo, RepositoryPath: foreignRepo,
	}})
	if err == nil {
		t.Fatalf("accepted foreign repository path as task worktree: %+v", manifest)
	}
}

func TestArchiveManifestRejectsDisappearedUntrackedPath(t *testing.T) {
	entries, err := archiveSourceManifestEntries(t.TempDir(), "?? disappeared.txt\x00")
	if err == nil {
		t.Fatalf("accepted disappeared untracked path without content identity: %+v", entries)
	}
}
