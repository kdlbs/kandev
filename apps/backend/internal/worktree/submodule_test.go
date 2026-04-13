package worktree

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func allowFileProtocol(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
}

func initRepoWithSubmodule(t *testing.T) (repoPath, submodulePath string) {
	t.Helper()
	allowFileProtocol(t)

	// Create the submodule repository first.
	submodulePath = t.TempDir()
	runGit(t, submodulePath, "init", "-b", "main")
	runGit(t, submodulePath, "config", "user.email", "test@example.com")
	runGit(t, submodulePath, "config", "user.name", "Test User")
	runGit(t, submodulePath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(submodulePath, "lib.txt"), []byte("library code\n"), 0644); err != nil {
		t.Fatalf("write lib.txt: %v", err)
	}
	runGit(t, submodulePath, "add", ".")
	runGit(t, submodulePath, "commit", "-m", "initial submodule commit")

	// Create the main repository.
	repoPath = t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "main.txt"), []byte("main code\n"), 0644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial main commit")

	// Add the submodule.
	runGit(t, repoPath, "submodule", "add", submodulePath, "libs/external")
	runGit(t, repoPath, "commit", "-m", "add submodule")

	return repoPath, submodulePath
}

func TestGetSubmodulePaths(t *testing.T) {
	repoPath, _ := initRepoWithSubmodule(t)

	paths, err := getSubmodulePaths(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("getSubmodulePaths() error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "libs/external" {
		t.Errorf("getSubmodulePaths() = %v, want [libs/external]", paths)
	}
}

func TestGetSubmodulePaths_NoSubmodules(t *testing.T) {
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial")

	paths, err := getSubmodulePaths(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("getSubmodulePaths() error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("getSubmodulePaths() = %v, want empty", paths)
	}
}

func TestGetSubmodulePaths_MultipleSubmodules(t *testing.T) {
	allowFileProtocol(t)
	// Create two submodule repos.
	sub1 := t.TempDir()
	runGit(t, sub1, "init", "-b", "main")
	runGit(t, sub1, "config", "user.email", "test@example.com")
	runGit(t, sub1, "config", "user.name", "Test User")
	runGit(t, sub1, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(sub1, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub1, "add", ".")
	runGit(t, sub1, "commit", "-m", "init")

	sub2 := t.TempDir()
	runGit(t, sub2, "init", "-b", "main")
	runGit(t, sub2, "config", "user.email", "test@example.com")
	runGit(t, sub2, "config", "user.name", "Test User")
	runGit(t, sub2, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(sub2, "b.txt"), []byte("b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub2, "add", ".")
	runGit(t, sub2, "commit", "-m", "init")

	// Main repo with both submodules.
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "root.txt"), []byte("root\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial")

	runGit(t, repoPath, "submodule", "add", sub1, "vendor/alpha")
	runGit(t, repoPath, "submodule", "add", sub2, "vendor/beta")
	runGit(t, repoPath, "commit", "-m", "add submodules")

	paths, err := getSubmodulePaths(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("getSubmodulePaths() error: %v", err)
	}
	sort.Strings(paths)
	if len(paths) != 2 || paths[0] != "vendor/alpha" || paths[1] != "vendor/beta" {
		t.Errorf("getSubmodulePaths() = %v, want [vendor/alpha vendor/beta]", paths)
	}
}

func TestGetSubmodulePaths_WorktreeNoCheckout(t *testing.T) {
	repoPath, _ := initRepoWithSubmodule(t)

	wtPath := filepath.Join(t.TempDir(), "wt")
	runGit(t, repoPath, "worktree", "add", "--no-checkout", "-b", "test-branch", wtPath, "main")

	paths, err := getSubmodulePaths(context.Background(), wtPath)
	if err != nil {
		t.Fatalf("getSubmodulePaths() in --no-checkout worktree error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "libs/external" {
		t.Errorf("getSubmodulePaths() = %v, want [libs/external]", paths)
	}
}

func TestInitSubmodules(t *testing.T) {
	repoPath, _ := initRepoWithSubmodule(t)

	// Create a worktree — submodule won't be initialized.
	wtPath := filepath.Join(t.TempDir(), "wt")
	runGit(t, repoPath, "worktree", "add", "-b", "sub-test", wtPath, "main")

	// Submodule directory should exist but be empty (not initialized).
	subDir := filepath.Join(wtPath, "libs", "external")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		t.Fatalf("submodule dir should exist: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("submodule dir should be empty before init, got %d entries", len(entries))
	}

	// Run initSubmodules.
	log := newTestLogger()
	m := &Manager{logger: log}
	m.initSubmodules(context.Background(), wtPath)

	// Submodule should now have content.
	content, err := os.ReadFile(filepath.Join(subDir, "lib.txt"))
	if err != nil {
		t.Fatalf("read submodule file after init: %v", err)
	}
	if strings.TrimSpace(string(content)) != "library code" {
		t.Errorf("submodule lib.txt = %q, want %q", string(content), "library code")
	}
}

func TestCreateWorktree_WithSubmodules(t *testing.T) {
	repoPath, _ := initRepoWithSubmodule(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "sub-task-1",
		SessionID:      "sub-session-1",
		TaskTitle:      "Submodule Test",
		RepositoryID:   "repo-sub",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Main file should be present.
	mainContent, err := os.ReadFile(filepath.Join(wt.Path, "main.txt"))
	if err != nil {
		t.Fatalf("read main.txt: %v", err)
	}
	if strings.TrimSpace(string(mainContent)) != "main code" {
		t.Errorf("main.txt = %q, want %q", string(mainContent), "main code")
	}

	// Submodule should be initialized with content.
	subContent, err := os.ReadFile(filepath.Join(wt.Path, "libs", "external", "lib.txt"))
	if err != nil {
		t.Fatalf("read submodule lib.txt: %v", err)
	}
	if strings.TrimSpace(string(subContent)) != "library code" {
		t.Errorf("submodule lib.txt = %q, want %q", string(subContent), "library code")
	}

	// Working tree should be clean.
	status := strings.TrimSpace(runGit(t, wt.Path, "status", "--porcelain"))
	if status != "" {
		t.Errorf("worktree is not clean: %s", status)
	}
}
