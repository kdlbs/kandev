package worktree

import (
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceAdoptionPathsSeparateRootFromNativeWorkingDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "tasks")
	root := filepath.Join(base, "task")
	old := filepath.Join(root, "repository")
	candidate := filepath.Join(root, "independent")
	slot := models.WorkspaceRecoveryInventorySlot{WorktreePath: old}
	for _, cwd := range []string{root, old} {
		selection := models.WorkspaceAdoptionSelection{Snapshot: models.WorkspaceRecoverySelectionSnapshot{TaskDirName: "task", WorkspacePath: cwd}}
		if !validWorkspaceAdoptionPaths(selection, slot, candidate, base) {
			t.Errorf("owned sibling candidate refused for working directory %q", cwd)
		}
		for _, foreign := range []string{filepath.Join(old, "nested"), filepath.Join(base, "foreign", "checkout"), old} {
			if validWorkspaceAdoptionPaths(selection, slot, foreign, base) {
				t.Errorf("invalid candidate %q accepted", foreign)
			}
		}
	}
	selection := models.WorkspaceAdoptionSelection{Snapshot: models.WorkspaceRecoverySelectionSnapshot{TaskDirName: "task", WorkspacePath: filepath.Join(base, "foreign")}}
	if validWorkspaceAdoptionPaths(selection, slot, candidate, base) {
		t.Fatal("foreign working directory accepted")
	}
}
