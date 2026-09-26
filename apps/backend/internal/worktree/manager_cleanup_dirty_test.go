package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDirtyWorktreeFiles(t *testing.T) {
	status := " M tracked.txt\x00?? untracked.txt\x00R  renamed-new.txt\x00renamed-old.txt\x00C  copied-new.txt\x00copied-old.txt\x00"
	got := parseDirtyWorktreeFiles(status)
	want := []string{"copied-new.txt", "renamed-new.txt", "tracked.txt", "untracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dirty files = %#v, want %#v", got, want)
	}
}

func TestParseDirtyWorktreeFilesPreservesPathWhitespace(t *testing.T) {
	status := "??  leading and trailing.txt \x00"
	got := parseDirtyWorktreeFiles(status)
	want := []string{" leading and trailing.txt "}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dirty files = %#v, want %#v", got, want)
	}
}

func TestInspectDirtyWorktreesFailsClosedWhenRequiredMetadataIsMissing(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	tests := []struct {
		name           string
		repositoryPath string
		worktreePath   string
	}{
		{name: "missing repository path", worktreePath: "/tmp/worktree"},
		{name: "missing worktree path", repositoryPath: "/tmp/repository"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := "wt-missing-metadata"
			_, err := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{{
				ID:             id,
				RepositoryPath: tt.repositoryPath,
				Path:           tt.worktreePath,
			}})
			if err == nil {
				t.Fatal("inspection succeeded without required worktree metadata")
			}
			if !strings.Contains(err.Error(), id) {
				t.Fatalf("inspection error = %v, want worktree id %q", err, id)
			}
		})
	}
}

func TestInspectDirtyWorktreesAllowsCheckoutRemovedDuringGitStatus(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	wt := createReferenceCleanupWorktree(t, mgr, "task-inspect-race", "session-inspect-race")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	shimDir := t.TempDir()
	gitShim := filepath.Join(shimDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$3" = "status" ]; then
  /bin/rm -rf -- "$KDEV_TEST_REMOVE_WORKTREE"
  exit 128
fi
exec %q "$@"
`, gitPath)
	if err := os.WriteFile(gitShim, []byte(script), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KDEV_TEST_REMOVE_WORKTREE", wt.Path)

	dirty, err := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{wt})
	if err != nil {
		t.Fatalf("inspection should tolerate a checkout removed by concurrent cleanup: %v", err)
	}
	if len(dirty) != 0 {
		t.Fatalf("dirty worktrees = %#v, want none for a removed checkout", dirty)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("git shim did not remove checkout, stat error = %v", err)
	}
}
