package worktree

import (
	"context"
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
