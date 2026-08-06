package process

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"

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

func TestComputeBaseCommit_CorrectsMergedDeletedParent(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	runGit(t, repoDir, "checkout", "-b", "feature/parent")
	writeFile(t, repoDir, "parent.txt", "parent work")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "parent work")
	parentTip := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--ff-only", "feature/parent")
	writeFile(t, repoDir, "main-after.txt", "main advanced")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "main advanced")
	runGit(t, repoDir, "push", "origin", "main")
	integrationBase := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "origin/main"))
	if integrationBase == parentTip {
		t.Fatal("test setup invalid: integration base must advance past the stale parent")
	}

	runGit(t, repoDir, "checkout", "-b", "feature/child", "origin/main")
	writeFile(t, repoDir, "child.txt", "child work")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "child work")

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	if got := wt.computeBaseCommit(context.Background(), "feature/parent"); got != integrationBase {
		t.Errorf("computeBaseCommit = %q, want corrected integration base %q", got, integrationBase)
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

// TestIsSafeGitRef_RejectsCommandInjectionShapes documents the unsafe-input
// boundary the SetBaseBranch / HTTP handlers rely on. The picker map is
// user-controlled and ends up in `exec.Command("git", …, ref)`; ref names
// starting with "-" or carrying shell metacharacters would be reinterpreted
// by git itself (`git --upload-pack=…`) or by the surrounding shell on
// non-CommandContext call sites.
func TestIsSafeGitRef_RejectsCommandInjectionShapes(t *testing.T) {
	safe := []string{"", "main", "origin/main", "feature/x", "release-1.2", "a_b.c", "v0.55.0"}
	unsafe := []string{
		"-upload-pack=evilcmd",
		"--exec=evil",
		"/leading-slash",
		"trailing-slash/",
		"with space",
		"semi;rm -rf",
		"pipe|cat",
		"back`ticks",
		"dollar$sign",
		"dot..dot",
		"ref@{0}",
		"new\nline",
		"tab\there",
	}
	for _, ref := range safe {
		if !IsSafeGitRef(ref) {
			t.Errorf("IsSafeGitRef(%q) = false, want true (safe input)", ref)
		}
	}
	for _, ref := range unsafe {
		if IsSafeGitRef(ref) {
			t.Errorf("IsSafeGitRef(%q) = true, want false (unsafe input)", ref)
		}
	}
}

// TestSetBaseBranch_RejectsUnsafeRefs confirms unsafe values downgrade to
// the no-override fallback at the boundary instead of being stored. Keeps
// the workspace-tracker contract honest even when a misbehaving caller
// (or an attacker who reached the agentctl HTTP surface) skipped the
// upstream sanitizer.
func TestSetBaseBranch_RejectsUnsafeRefs(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetBaseBranch("-upload-pack=evil")
	if got := wt.BaseBranch(); got != "" {
		t.Errorf("BaseBranch after unsafe SetBaseBranch = %q, want empty", got)
	}
	wt.SetBaseBranch("main")
	if got := wt.BaseBranch(); got != "main" {
		t.Errorf("BaseBranch after safe SetBaseBranch = %q, want %q", got, "main")
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

// Regression: resolveBaseBranch fell through to the integration-branch list
// silently. When a task's base branch never reached the tracker, the diff stat
// was computed against origin/master — a wrong-but-plausible number with
// nothing in the logs to attribute it. The two fallback reasons need different
// fixes (propagation vs a branch that no longer exists in git), so they must be
// distinguishable without re-running git by hand.
func TestResolveBaseBranchWithReason(t *testing.T) {
	ctx := context.Background()

	t.Run("stored branch resolves", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		runGit(t, repoDir, "checkout", "-b", "develop")
		runGit(t, repoDir, "checkout", "main")

		wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
		wt.SetBaseBranch("develop")

		got := wt.resolveBaseBranchWithReason(ctx)
		if got.ref != "develop" {
			t.Errorf("ref = %q, want %q", got.ref, "develop")
		}
		if got.reason != baseBranchStored {
			t.Errorf("reason = %v, want baseBranchStored", got.reason)
		}
	})

	t.Run("no stored branch falls back", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		wt := NewWorkspaceTracker(repoDir, newTestLogger(t))

		got := wt.resolveBaseBranchWithReason(ctx)
		if got.ref != "origin/main" {
			t.Errorf("ref = %q, want %q", got.ref, "origin/main")
		}
		if got.reason != baseBranchFallbackNoStored {
			t.Errorf("reason = %v, want baseBranchFallbackNoStored", got.reason)
		}
		if got.stored != "" {
			t.Errorf("stored = %q, want empty", got.stored)
		}
	})

	// A base branch that is recorded but missing from git is a different
	// failure than one that never arrived — this is the case where the branch
	// was deleted or never fetched, not a propagation gap.
	t.Run("stored branch that does not exist falls back and is attributable", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
		wt.SetBaseBranch("features/never-fetched")

		got := wt.resolveBaseBranchWithReason(ctx)
		if got.ref != "origin/main" {
			t.Errorf("ref = %q, want %q", got.ref, "origin/main")
		}
		if got.reason != baseBranchFallbackStoredUnresolved {
			t.Errorf("reason = %v, want baseBranchFallbackStoredUnresolved", got.reason)
		}
		if got.stored != "features/never-fetched" {
			t.Errorf("stored = %q, want the recorded branch", got.stored)
		}
	})

	// resolveBaseBranch keeps its exact contract — only observability is added.
	t.Run("resolveBaseBranch returns the same ref", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
		wt.SetBaseBranch("features/never-fetched")

		if got := wt.resolveBaseBranch(ctx); got != wt.resolveBaseBranchWithReason(ctx).ref {
			t.Errorf("resolveBaseBranch diverged from resolveBaseBranchWithReason")
		}
	})
}

// The reason enum only pays off if it reaches the operator, and only if it does
// so at a level and with fields that identify *which* repository fell back to
// *which* candidate. resolveBaseBranch also runs on every status poll, so an
// unsuppressed log would bury the one event that matters under thousands of
// identical repeats — the suppression is part of the contract, not an
// optimisation.
func TestResolveBaseBranchLogsFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("single-repo fallback names the repository", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTracker(repoDir, log)

		wt.resolveBaseBranch(ctx)

		entries := observed.FilterMessage(
			"no base branch recorded for workspace, using integration fallback for diff stats").All()
		if len(entries) != 1 {
			t.Fatalf("got %d fallback log entries, want 1", len(entries))
		}
		if got := entries[0].ContextMap()["repository"]; got != filepath.Base(repoDir) {
			t.Errorf("repository field = %v, want %q", got, filepath.Base(repoDir))
		}
	})

	// A missing base branch is ordinary (legacy tasks, external branches), so it
	// stays at debug and must not cry wolf at warn.
	t.Run("no stored branch logs at debug with repository and candidate", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)

		wt.resolveBaseBranch(ctx)

		entries := observed.FilterMessage(
			"no base branch recorded for workspace, using integration fallback for diff stats").All()
		if len(entries) != 1 {
			t.Fatalf("got %d fallback log entries, want 1", len(entries))
		}
		if entries[0].Level != zapcore.DebugLevel {
			t.Errorf("level = %v, want debug", entries[0].Level)
		}
		fields := entries[0].ContextMap()
		if fields["repository"] != "frontend" {
			t.Errorf("repository field = %v, want frontend", fields["repository"])
		}
		if fields["candidate"] != "origin/main" {
			t.Errorf("candidate field = %v, want origin/main", fields["candidate"])
		}
	})

	// A base branch that was recorded but does not resolve is an anomaly the
	// operator has to see without turning on debug logging.
	t.Run("stored branch that does not resolve warns and names the branch", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)
		wt.SetBaseBranch("features/never-fetched")

		wt.resolveBaseBranch(ctx)

		entries := observed.FilterMessage(
			"recorded base branch does not resolve in git, diff stats fall back to an integration branch").All()
		if len(entries) != 1 {
			t.Fatalf("got %d fallback log entries, want 1", len(entries))
		}
		if entries[0].Level != zapcore.WarnLevel {
			t.Errorf("level = %v, want warn", entries[0].Level)
		}
		fields := entries[0].ContextMap()
		if fields["repository"] != "frontend" {
			t.Errorf("repository field = %v, want frontend", fields["repository"])
		}
		if fields["stored_base_branch"] != "features/never-fetched" {
			t.Errorf("stored_base_branch field = %v, want features/never-fetched", fields["stored_base_branch"])
		}
		if fields["candidate"] != "origin/main" {
			t.Errorf("candidate field = %v, want origin/main", fields["candidate"])
		}
	})

	t.Run("an unchanged outcome is logged once", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)
		wt.SetBaseBranch("features/never-fetched")

		for range 5 {
			wt.resolveBaseBranch(ctx)
		}

		if got := observed.Len(); got != 1 {
			t.Fatalf("got %d log entries across 5 identical resolutions, want 1: %+v", got, observed.All())
		}
	})

	// Suppression keys off the outcome, not "have I ever logged" — a tracker
	// whose resolution changes has to report the new one, or the log stops
	// tracking reality after the first fallback.
	t.Run("a changed outcome is logged again", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)

		wt.resolveBaseBranch(ctx)
		wt.SetBaseBranch("features/never-fetched")
		wt.resolveBaseBranch(ctx)

		if got := observed.Len(); got != 2 {
			t.Fatalf("got %d log entries, want 2 (one per distinct outcome): %+v", got, observed.All())
		}
	})

	t.Run("different unresolved stored branches are logged separately", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)

		wt.SetBaseBranch("features/first-missing")
		wt.resolveBaseBranch(ctx)
		wt.SetBaseBranch("features/second-missing")
		wt.resolveBaseBranch(ctx)

		if got := observed.Len(); got != 2 {
			t.Fatalf("got %d log entries, want 2 for distinct stored branches: %+v", got, observed.All())
		}
	})

	t.Run("fallback after a successful stored resolution is logged again", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		runGit(t, repoDir, "checkout", "-b", "develop")
		runGit(t, repoDir, "checkout", "main")

		log, observed := newObservedTestLogger(t)
		wt := NewWorkspaceTrackerForRepo(repoDir, "frontend", log)

		wt.SetBaseBranch("features/never-fetched")
		wt.resolveBaseBranch(ctx)
		wt.SetBaseBranch("develop")
		wt.resolveBaseBranch(ctx)
		wt.SetBaseBranch("features/never-fetched")
		wt.resolveBaseBranch(ctx)

		if got := observed.Len(); got != 2 {
			t.Fatalf("got %d log entries, want 2 across fallback, stored, fallback: %+v", got, observed.All())
		}
	})
}
