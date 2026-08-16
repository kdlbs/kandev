package delivery_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/delivery"
)

// fakeCheckoutResolver satisfies delivery.CheckoutResolver for tests
// without pulling in task/service. calls, when non-nil, records how many
// times ResolveRepositoryLocalPath was invoked, so a test can assert a
// rejection happened before the checkout was even resolved rather than
// merely that Check ended up Errored (which a later, unrelated failure could
// also produce).
type fakeCheckoutResolver struct {
	path  string
	err   error
	calls *int
}

func (f fakeCheckoutResolver) ResolveRepositoryLocalPath(_ context.Context, _ string) (string, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.path, f.err
}

// runGit runs a git command in dir for test repo setup, failing the test
// on error. This is test fixture setup, not the production seam under
// test (which goes through delivery.AncestryChecker).
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out.String())
	}
	return strings.TrimSpace(out.String())
}

// newAncestryTestRepo creates a bare "origin" plus a working checkout with
// a remote-tracking branch, so refs/remotes/origin/<default_branch>
// resolves the way a real clone would.
func newAncestryTestRepo(t *testing.T) (checkoutPath, defaultBranch string) {
	t.Helper()
	origin := t.TempDir()
	runGit(t, origin, "init", "--bare", "-b", "main")

	work := t.TempDir()
	runGit(t, work, "init", "-b", "main")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "test")
	runGit(t, work, "remote", "add", "origin", origin)
	writeFile(t, work, "README.md", "hello")
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "initial")
	runGit(t, work, "push", "origin", "main")
	runGit(t, work, "fetch", "origin")

	return work, "main"
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestAncestryChecker_PositiveResult(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)
	head := runGit(t, work, "rev-parse", "HEAD")

	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", defaultBranch, head)

	if !out.Attempted || out.Errored {
		t.Fatalf("out = %+v, want Attempted, not Errored", out)
	}
	if !out.Positive {
		t.Fatal("HEAD of the default branch must be an ancestor of itself")
	}
}

func TestAncestryChecker_NegativeResultIsNotAnError(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)
	runGit(t, work, "checkout", "-b", "feature")
	writeFile(t, work, "feature.txt", "wip")
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work")
	featureHead := runGit(t, work, "rev-parse", "HEAD")

	// Advance main independently so the feature commit is not an ancestor.
	runGit(t, work, "checkout", defaultBranch)
	writeFile(t, work, "main.txt", "unrelated")
	runGit(t, work, "add", "main.txt")
	runGit(t, work, "commit", "-m", "unrelated main commit")
	runGit(t, work, "push", "origin", defaultBranch)
	runGit(t, work, "fetch", "origin")

	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", defaultBranch, featureHead)

	if out.Errored {
		t.Fatalf("a negative ancestry result must not be an error: %+v", out)
	}
	if out.Positive {
		t.Fatal("an unmerged feature commit must not be an ancestor of main")
	}
}

func TestAncestryChecker_LocalBranchFallbackWhenRemoteTrackingRefMissing(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)
	// Simulate a repository with no remote-tracking ref at all (never
	// fetched, or a purely local checkout).
	runGit(t, work, "update-ref", "-d", "refs/remotes/origin/"+defaultBranch)
	head := runGit(t, work, "rev-parse", "HEAD")

	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", defaultBranch, head)

	if out.Errored {
		t.Fatalf("expected the local branch fallback to succeed, got errored: %+v", out)
	}
	if !out.Positive {
		t.Fatal("HEAD must be an ancestor of its own local default branch")
	}
}

func TestAncestryChecker_NeitherRefResolvesIsAnError(t *testing.T) {
	work, _ := newAncestryTestRepo(t)
	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", "does-not-exist", "HEAD")

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true", out)
	}
}

// TestAncestryChecker_FlagLikeCommitIsRejectedBeforeReachingGit covers
// Review round 1, finding #7: task_session_git_snapshots.head_commit is
// not format-validated anywhere upstream of this seam, and
// apps/backend/internal/common/subproc/git_command.go documents that
// callers are responsible for validating a value before it reaches a git
// subprocess as raw argv. A commit value beginning with "-" previously
// went straight to `git merge-base --is-ancestor <commit> <ref>` with no
// `--` separator, so it could be parsed as a git option rather than a
// revision. Check must now reject a non-SHA-shaped commit before it ever
// reaches a git subprocess, rather than relying on git's own error
// handling for the malformed value.
func TestAncestryChecker_FlagLikeCommitIsRejectedBeforeReachingGit(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)

	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", defaultBranch, "--all")

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true (a flag-like commit must never reach the git subprocess)", out)
	}
}

// TestAncestryChecker_FlagLikeDefaultBranchIsRejectedBeforeReachingGit
// covers Review round 2, finding #1 (SEC-005): repositories.default_branch
// is attacker-reachable through ordinary repository create/update requests
// and was never validated before reaching refExists as raw argv. Round 1's
// finding #7 guarded the commit argument with looksLikeCommitSHA but left
// defaultBranch unguarded — a flag-like value here risks option-injection
// against `git rev-parse --verify --quiet <value>^{commit}`, which cannot
// take a `--` separator (it would turn the argument into a pathspec), so
// the fix must be validation before the call, mirroring looksLikeCommitSHA's
// treatment of commit.
//
// Review round 3, finding #3: this test originally only asserted
// Errored=true, which is phantom-green — "-x" already produces Errored=true
// via the pre-existing "neither ref resolves" path (refExists's rev-parse
// calls both exit non-zero for an unrecognized flag), identically on
// pre-fix code, so the assertion could not tell "rejected before reaching
// git" apart from "reached git and git errored anyway". Asserting the
// checkout was never even resolved makes the test actually discriminate:
// pre-fix, Check calls a.Checkout.ResolveRepositoryLocalPath unconditionally
// before ever touching defaultBranch.
func TestAncestryChecker_FlagLikeDefaultBranchIsRejectedBeforeReachingGit(t *testing.T) {
	work, _ := newAncestryTestRepo(t)
	head := runGit(t, work, "rev-parse", "HEAD")

	calls := 0
	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work, calls: &calls}}
	out := checker.Check(context.Background(), "repo-1", "-x", head)

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true (a flag-like default branch must never reach the git subprocess)", out)
	}
	if calls != 0 {
		t.Fatalf("ResolveRepositoryLocalPath was called %d time(s), want 0 "+
			"(the guard must reject defaultBranch before resolving the checkout at all)", calls)
	}
}

// TestAncestryChecker_CommitSHALikeDefaultBranchIsRejectedBeforeReachingGit
// covers Review round 3, finding #2: IsValidBranchName's allowlist regex
// admits a bare commit SHA (pure hex, no leading dash, no "~"/"^"), which is
// not a real branch name but resolves via refExists's local-fallback
// rev-parse to whatever commit that SHA names — the silent wrong-commit
// failure mode the guard exists to close, not merely a rejection based on
// "no such ref".
func TestAncestryChecker_CommitSHALikeDefaultBranchIsRejectedBeforeReachingGit(t *testing.T) {
	work, _ := newAncestryTestRepo(t)
	head := runGit(t, work, "rev-parse", "HEAD")

	calls := 0
	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work, calls: &calls}}
	// head is a real, resolvable commit SHA in this checkout — if it were
	// treated as a branch name instead of rejected, resolveDefaultBranchRef
	// would resolve it successfully rather than failing "does not exist".
	out := checker.Check(context.Background(), "repo-1", head, head)

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true (a commit-SHA-shaped default branch must never reach the git subprocess)", out)
	}
	if calls != 0 {
		t.Fatalf("ResolveRepositoryLocalPath was called %d time(s), want 0", calls)
	}
}

// TestAncestryChecker_SymbolicRefDefaultBranchIsRejectedBeforeReachingGit
// covers the second half of Review round 3, finding #2: git's well-known
// symbolic refs (HEAD, ORIG_HEAD, FETCH_HEAD, MERGE_HEAD) are also pure
// alnum/underscore and pass IsValidBranchName, but none is a real branch
// name — HEAD in particular always resolves in a non-empty checkout, to
// whatever commit is currently checked out, independent of the caller's
// intended default branch.
func TestAncestryChecker_SymbolicRefDefaultBranchIsRejectedBeforeReachingGit(t *testing.T) {
	work, _ := newAncestryTestRepo(t)
	head := runGit(t, work, "rev-parse", "HEAD")

	for _, ref := range []string{"HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD"} {
		t.Run(ref, func(t *testing.T) {
			calls := 0
			checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work, calls: &calls}}
			out := checker.Check(context.Background(), "repo-1", ref, head)

			if !out.Errored || out.Positive {
				t.Fatalf("out = %+v, want Errored=true (%q must never reach the git subprocess as a default branch)", out, ref)
			}
			if calls != 0 {
				t.Fatalf("ResolveRepositoryLocalPath was called %d time(s), want 0", calls)
			}
		})
	}
}

// TestAncestryChecker_RevisionSyntaxDefaultBranchIsRejectedBeforeReachingGit
// covers the second, more dangerous consequence named in finding #1: a
// value like "main~1" is not flag-like, but git parses it as revision syntax
// (a parent-commit modifier) rather than a literal ref name. This is not a
// theoretical concern — with a real second commit on the default branch,
// "refs/remotes/origin/main~1^{commit}" resolves successfully (exit 0) to
// the PARENT commit, silently, with no error at all: the exact
// "nonexistent ref resolves to the wrong commit entirely" failure mode the
// finding describes. A repo with only one commit cannot reproduce this
// (main~1 has no parent to resolve to, so it fails safe by accident), so
// this test seeds a second commit specifically to exercise the real defect.
func TestAncestryChecker_RevisionSyntaxDefaultBranchIsRejectedBeforeReachingGit(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)
	writeFile(t, work, "second.txt", "second commit")
	runGit(t, work, "add", "second.txt")
	runGit(t, work, "commit", "-m", "second commit")
	runGit(t, work, "push", "origin", defaultBranch)
	runGit(t, work, "fetch", "origin")
	head := runGit(t, work, "rev-parse", "HEAD")

	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	out := checker.Check(context.Background(), "repo-1", defaultBranch+"~1", head)

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true (revision syntax must never reach the git subprocess as a literal ref, "+
			"even when it happens to resolve to a real, but wrong, commit)", out)
	}
}

// TestAncestryChecker_CheckoutResolverErrorIsAnAncestryError is Review
// round 2, finding #5: the original version of this test passed commit
// "HEAD", which looksLikeCommitSHA rejects before Check ever reaches
// a.Checkout.ResolveRepositoryLocalPath (ancestry.go's very first guard) —
// so it was phantom-green, passing for the commit-format-validation
// reason covered elsewhere, never exercising the checkout-resolver error
// path its name claims. This version uses a real-looking 40-hex commit
// SHA and a valid branch name so Check's earlier guards let the call
// through, and asserts via calls that the resolver was actually invoked.
func TestAncestryChecker_CheckoutResolverErrorIsAnAncestryError(t *testing.T) {
	calls := 0
	checker := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{err: errTestCheckout, calls: &calls}}
	out := checker.Check(context.Background(), "repo-1", "main", strings.Repeat("a", 40))

	if !out.Errored || out.Positive {
		t.Fatalf("out = %+v, want Errored=true (no readable local checkout)", out)
	}
	if calls != 1 {
		t.Fatalf("ResolveRepositoryLocalPath calls = %d, want 1 (Check must actually reach the checkout resolver)", calls)
	}
}

var errTestCheckout = errors.New("repository local path is empty")
