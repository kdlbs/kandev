//go:build linux || darwin

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeGitRepositoryDoesNotFollowReplacedDirectoryPath(t *testing.T) {
	parent := t.TempDir()
	staging := filepath.Join(parent, "staging")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatalf("Mkdir staging: %v", err)
	}
	directory, err := openLocalRepositoryDirectory(staging)
	if err != nil {
		t.Fatalf("openLocalRepositoryDirectory: %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })

	openedDirectory := filepath.Join(parent, "opened-directory")
	if err := os.Rename(staging, openedDirectory); err != nil {
		t.Fatalf("Rename staging: %v", err)
	}
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatalf("Mkdir replacement: %v", err)
	}

	if initErr := initializeGitRepository(context.Background(), staging, directory); initErr != nil {
		t.Fatalf("initializeGitRepository: %v", initErr)
	}

	if info, err := os.Stat(filepath.Join(openedDirectory, ".git")); err != nil || !info.IsDir() {
		t.Fatalf("opened directory .git: info=%v error=%v", info, err)
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		t.Fatalf("ReadDir replacement: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("replacement entries = %+v, want none", entries)
	}
}
