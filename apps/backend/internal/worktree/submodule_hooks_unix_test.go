//go:build unix

package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

// TestInitSubmodulesRunsNoHooksEvenWhenParentOptsIn proves a submodule's own
// hooks stay off when the parent repository's hooks opt-in is on the context.
func TestInitSubmodulesRunsNoHooksEvenWhenParentOptsIn(t *testing.T) {
	repoPath, _ := initRepoWithSubmodule(t)
	marker := filepath.Join(t.TempDir(), "submodule-hook-ran")
	// Every clone made during the update is seeded with a post-checkout hook.
	template := t.TempDir()
	if err := os.MkdirAll(filepath.Join(template, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\n: > '" + marker + "'\n"
	if err := os.WriteFile(filepath.Join(template, "hooks", "post-checkout"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_TEMPLATE_DIR", template)

	wtPath := filepath.Join(t.TempDir(), "wt")
	runGit(t, repoPath, "worktree", "add", "-b", "sub-hooks", wtPath, "main")
	_ = os.Remove(marker) // the worktree add itself is not under test

	m := &Manager{logger: newTestLogger()}
	m.initSubmodules(subproc.WithGitHooks(context.Background(), true), wtPath)

	if _, err := os.Stat(filepath.Join(wtPath, "libs", "external", "lib.txt")); err != nil {
		t.Fatalf("submodule was not initialized: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a submodule hook ran although hooks are never enabled for submodules")
	}
}
