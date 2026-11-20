package process

import (
	"path/filepath"
	"testing"
)

func TestScanRepositorySubdirsSkipsExcludedDetachedRoot(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	detached := filepath.Join(taskRoot, "primary-feature")
	initGitRepoAt(t, primary)
	initGitRepoAt(t, detached)
	children := scanRepositorySubdirs(taskRoot, nil, []string{canonicalSourceRoot(t, detached)})
	if len(children) != 1 || children[0].path != primary {
		t.Fatalf("children = %+v, want only the still-attached primary root", children)
	}
}
