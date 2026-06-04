package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// TestResolveBaseBranch_StoredOverridesFallback verifies the task-recorded
// base_branch wins over the hardcoded origin/main → master priority list.
func TestResolveBaseBranch_StoredOverridesFallback(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "develop")
	writeFile(t, repoDir, "dev.txt", "dev")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "develop work")
	runGit(t, repoDir, "checkout", "main")

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("develop")

	if got := wt.resolveBaseBranch(context.Background()); got != "develop" {
		t.Fatalf("resolveBaseBranch = %q, want %q", got, "develop")
	}
}

// TestResolveBaseBranch_EmptyFallsBack confirms legacy tasks (no stored value)
// still resolve to the existing integration-branch list — preserving today's
// behaviour for migrated-from-singular tasks and external branches.
func TestResolveBaseBranch_EmptyFallsBack(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))

	if got := wt.resolveBaseBranch(context.Background()); got != "origin/main" {
		t.Fatalf("resolveBaseBranch = %q, want %q (fallback)", got, "origin/main")
	}
}

// TestResolveBaseBranch_PrefersOriginPrefix mirrors the long-standing
// computeMergeBase priority used by agentctl/server/api/git.go: when both
// `origin/<name>` and `<name>` exist in the workspace, the upstream remote
// ref wins. This keeps the task-card stats and the commits panel anchored
// to the same merge-base — without it, a stale local ref would produce a
// `+N -M` count that disagrees with the commit list rendered against
// origin.
func TestResolveBaseBranch_PrefersOriginPrefix(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Push a release branch upstream so origin/release exists, then move the
	// LOCAL release ref forward to a divergent SHA. resolveBaseBranch must
	// pick origin/release, not the advanced-local fork.
	runGit(t, repoDir, "checkout", "-b", "release")
	writeFile(t, repoDir, "release.txt", "rel")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "release base")
	runGit(t, repoDir, "push", "origin", "release")
	writeFile(t, repoDir, "release.txt", "rel-local-ahead")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "local-only ahead of origin/release")
	runGit(t, repoDir, "checkout", "main")

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("release")

	if got := wt.resolveBaseBranch(context.Background()); got != "origin/release" {
		t.Fatalf("resolveBaseBranch = %q, want %q (origin must win over stale local)", got, "origin/release")
	}
}

// TestComputeBaseCommit_FallsBackToTipWhenNoMergeBase covers the
// unrelated-histories case (e.g. local rebase-backup branches, freshly
// imported repos): merge-base returns exit 1, so we fall back to the
// branch tip as the diff anchor. Without this fallback BaseCommit goes
// empty and the task-card stats silently switch to summing per-file
// additions while the commits panel returns "last N HEAD commits" — the
// `+1 -0 vs 100 unrelated commits` mismatch the picker triggered on a
// backup-before-rebase pick.
func TestComputeBaseCommit_FallsBackToTipWhenNoMergeBase(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Build an orphan branch (no parent) so merge-base with HEAD fails.
	runGit(t, repoDir, "checkout", "--orphan", "unrelated")
	runGit(t, repoDir, "rm", "-rf", ".")
	writeFile(t, repoDir, "orphan.txt", "orphan")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "orphan root")
	orphanTip := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "main")

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	got := wt.computeBaseCommit(context.Background(), "unrelated")
	if got != orphanTip {
		t.Errorf("computeBaseCommit = %q, want %q (orphan tip fallback)", got, orphanTip)
	}
}

// TestResolveBaseBranch_InvalidStoredFallsThrough handles tasks whose recorded
// base_branch no longer exists locally (deleted, renamed, never fetched). The
// resolver must continue to the hardcoded list instead of returning a ref git
// cannot verify.
func TestResolveBaseBranch_InvalidStoredFallsThrough(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("does-not-exist-anywhere")

	if got := wt.resolveBaseBranch(context.Background()); got != "origin/main" {
		t.Fatalf("resolveBaseBranch = %q, want %q (invalid stored ref must fall through)", got, "origin/main")
	}
}

// TestResolveAheadBehindRef_StoredWins mirrors the diff-stat resolver for
// ahead/behind counts. Same "task-stored wins" contract — without it the UI's
// Pull/Push indicator would always count against main even for develop-based
// tasks.
func TestResolveAheadBehindRef_StoredWins(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "develop")
	writeFile(t, repoDir, "dev.txt", "dev")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "develop work")
	runGit(t, repoDir, "checkout", "main")

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("develop")

	if got := wt.resolveAheadBehindRef(context.Background()); got != "develop" {
		t.Fatalf("resolveAheadBehindRef = %q, want %q", got, "develop")
	}
}

// TestGetGitBranchInfo_StoredBaseDrivesBaseCommit is the full-pipeline check.
// When the tracker has a stored base_branch the BaseCommit on the resulting
// update is the merge-base against THAT ref, not the integration branch —
// directly reproducing the inflated-counts bug the user reported.
func TestGetGitBranchInfo_StoredBaseDrivesBaseCommit(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// 'develop' starts at the initial commit; add commits on main that the
	// task branch (off develop) should not see in its diff.
	runGit(t, repoDir, "branch", "develop")
	writeFile(t, repoDir, "main-only.txt", "main work")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "main-only commit")
	mainTip := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Branch the task off develop (NOT off main). HEAD will sit on the task
	// branch; merge-base(develop, HEAD) must resolve to develop's tip.
	runGit(t, repoDir, "checkout", "develop")
	runGit(t, repoDir, "checkout", "-b", "task-branch")
	writeFile(t, repoDir, "task.txt", "task work")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "task commit")
	wantBase := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "develop"))

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("develop")

	update := types.GitStatusUpdate{}
	if err := wt.getGitBranchInfo(context.Background(), &update); err != nil {
		t.Fatalf("getGitBranchInfo failed: %v", err)
	}

	if update.BaseCommit != wantBase {
		t.Errorf("BaseCommit = %q, want %q (merge-base with stored develop)", update.BaseCommit, wantBase)
	}
	if update.BaseCommit == mainTip {
		t.Errorf("BaseCommit equals main tip; stored base_branch was ignored")
	}
}

// TestLookupBaseBranch_FallbackToEmptyKey covers the map lookup the process
// manager uses to hand each tracker its base branch. Per-repo entry wins;
// missing per-repo falls back to the empty-key (single-repo) entry; legacy
// tasks with neither return empty so the fallback list applies.
func TestLookupBaseBranch_FallbackToEmptyKey(t *testing.T) {
	tests := []struct {
		name     string
		branches map[string]string
		repoName string
		want     string
	}{
		{"nil map", nil, "any", ""},
		{"empty key matches root", map[string]string{"": "main"}, "", "main"},
		{"empty key as fallback for unknown repo", map[string]string{"": "main"}, "missing-repo", "main"},
		{"per-repo wins over empty", map[string]string{"": "main", "repo-a": "develop"}, "repo-a", "develop"},
		{"per-repo only, missing entry returns empty", map[string]string{"repo-a": "develop"}, "repo-b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lookupBaseBranch(tt.branches, tt.repoName); got != tt.want {
				t.Errorf("lookupBaseBranch = %q, want %q", got, tt.want)
			}
		})
	}
}
