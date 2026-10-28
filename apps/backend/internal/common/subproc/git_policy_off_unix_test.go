//go:build unix

package subproc

import (
	"context"
	"testing"
)

func TestManagedGitOffRunsRepositorySuppliedCode(t *testing.T) {
	setIsolationForTest(t, false)
	repo := newPolicyRepo(t)
	mustGit(context.Background(), t, repo.dir, "commit", "--allow-empty", "-m", "plain")
	if !repo.marked() {
		t.Fatal("with the mode off the repository's own hook must run, as it did before isolation existed")
	}
}
