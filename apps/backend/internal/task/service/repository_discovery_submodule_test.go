package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitLocalRepositoryPath_AcceptsReciprocalSubmoduleMetadata(t *testing.T) {
	parent := canonicalTempDir(t)
	repoPath := filepath.Join(parent, "module")
	gitDir := filepath.Join(parent, "metadata", "modules", "module")
	worktree, err := filepath.Rel(gitDir, repoPath)
	if err != nil {
		t.Fatalf("Rel worktree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("MkdirAll git metadata: %v", err)
	}
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[Core]\n  WorkTree = "+worktree+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("WriteFile HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("0000000000000000000000000000000000000000\n"), 0o600); err != nil {
		t.Fatalf("WriteFile main ref: %v", err)
	}

	resolved, branch, err := resolveExplicitLocalRepositoryPath(repoPath)
	if err != nil {
		t.Fatalf("resolveExplicitLocalRepositoryPath: %v", err)
	}
	if resolved != repoPath {
		t.Fatalf("resolved path = %q, want %q", resolved, repoPath)
	}
	if branch != "main" {
		t.Fatalf("default branch = %q, want main", branch)
	}
}

func TestParseGitConfigCoreWorktree(t *testing.T) {
	for _, test := range []struct {
		name, config, want string
		wantErr            bool
	}{
		{"case and whitespace", "[Core]\n WorkTree = ../module \n", "../module", false},
		{"compact", "[core]\nworktree=module\n", "module", false},
		{"outside core", "worktree=wrong\n[other]\nworktree=also-wrong\n", "", true},
		{"empty", "[core]\nworktree = \n", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseGitConfigCoreWorktree(test.config)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parse = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestResolveExplicitLocalRepositoryPath_RejectsMismatchedSubmoduleWorktree(t *testing.T) {
	parent := canonicalTempDir(t)
	repoPath := filepath.Join(parent, "module")
	gitDir := filepath.Join(parent, "metadata", "modules", "module")
	other := filepath.Join(parent, "other")
	worktree, err := filepath.Rel(gitDir, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{filepath.Join(repoPath, ".git"): "gitdir: " + gitDir, filepath.Join(gitDir, "config"): "[core]\nworktree=" + worktree, filepath.Join(gitDir, "HEAD"): "ref: refs/heads/main"} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err = resolveExplicitLocalRepositoryPath(repoPath)
	if !errors.Is(err, ErrInvalidRepositoryPath) {
		t.Fatalf("error = %v", err)
	}
}
