package delivery

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
)

// commitSHAPattern matches a plausible git object name: hex digits only,
// long enough to rule out a bare option-like fragment and short enough to
// cover both SHA-1 (40 hex chars) and a future SHA-256 repository (64).
// commit comes from task_session_git_snapshots.head_commit, which is not
// format-validated anywhere upstream of this seam.
var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// looksLikeCommitSHA rejects a commit value before it ever reaches a git
// subprocess as raw argv. apps/backend/internal/common/subproc/git_command.go
// documents that callers are responsible for validating user-controlled
// refs before they reach that seam — a value beginning with "-" could
// otherwise be parsed as a git option rather than a revision (Review
// round 1, finding #7).
func looksLikeCommitSHA(commit string) bool {
	return commitSHAPattern.MatchString(commit)
}

// CheckoutResolver is the narrow, read-only, single-method port the
// ancestry check depends on for the repository's local checkout path
// (spec "Which checkout"). It is satisfied by
// task/service.Service.ResolveRepositoryLocalPath, wired in at
// construction so this package never depends on the concrete task
// service type or duplicates its canonicalization/containment checks.
type CheckoutResolver interface {
	ResolveRepositoryLocalPath(ctx context.Context, repositoryID string) (string, error)
}

// AncestryCallTimeout is the fixed per-call timeout (spec "Boundary
// values").
const AncestryCallTimeout = 10 * time.Second

// AncestryChecker runs the ancestry check through the class-aware git
// subprocess admission pool at subproc.GitBackground — the sweep is
// background work and must not contend with user-facing git (spec
// "Concurrency", "Ancestry admission class").
type AncestryChecker struct {
	Checkout CheckoutResolver
}

// Check resolves the repository's checkout, resolves the default branch
// ref (remote-tracking first, falling back to the local branch), and runs
// `git merge-base --is-ancestor commit ref`. It never returns a negative
// result as an error under Errored; Errored is set only when the check
// could not be performed at all — no readable local checkout, neither
// ref resolves, or the git call itself fails or times out (spec "Failure
// modes").
func (a *AncestryChecker) Check(ctx context.Context, repositoryID, defaultBranch, commit string) AncestryOutcome {
	out := AncestryOutcome{Attempted: true, Commit: commit}
	if !looksLikeCommitSHA(commit) {
		out.Errored = true
		return out
	}
	// repositories.default_branch is attacker-reachable through ordinary
	// repository create/update requests and is not format-validated
	// upstream of this seam either (Review round 2, finding #1). A
	// flag-like value risks option-injection against refExists's rev-parse
	// call, which cannot take a "--" separator (it would turn the argument
	// into a pathspec). More dangerously, a value like "main~1" is not
	// flag-like at all but is valid git revision syntax: appended to a ref
	// that does exist (e.g. "refs/remotes/origin/main~1"), it resolves
	// successfully to the wrong commit with no error, rather than failing
	// as "does not exist" — defense-in-depth alongside the ingestion-time
	// validation in task/service.applyRepositoryUpdates /
	// CreateRepository, mirroring looksLikeCommitSHA's treatment of
	// commit above. IsValidDefaultBranchName's allowlist admits bare commit
	// SHAs (both pure alnum/underscore, no leading dash, no "~"/"^"), which
	// is not a real branch name and resolves via refExists's local-fallback
	// rev-parse to a commit that is not what the caller's default_branch
	// value names — the same silent wrong-commit failure mode this guard
	// exists to close (Review round 3, finding #2). Git's well-known
	// symbolic refs (HEAD and friends) are rejected by
	// IsValidDefaultBranchName itself.
	if !securityutil.IsValidDefaultBranchName(defaultBranch) || looksLikeCommitSHA(defaultBranch) {
		out.Errored = true
		return out
	}
	if a.Checkout == nil {
		out.Errored = true
		return out
	}
	path, err := a.Checkout.ResolveRepositoryLocalPath(ctx, repositoryID)
	if err != nil {
		out.Errored = true
		return out
	}
	ref, err := resolveDefaultBranchRef(ctx, path, defaultBranch)
	if err != nil {
		out.Errored = true
		return out
	}
	positive, err := isAncestor(ctx, path, commit, ref)
	if err != nil {
		out.Errored = true
		return out
	}
	out.Positive = positive
	return out
}

// resolveDefaultBranchRef implements spec "Which default branch":
// refs/remotes/origin/<default_branch>, falling back to the local branch
// <default_branch> when the remote-tracking ref genuinely does not exist.
// A failure to CHECK the remote ref (as opposed to a confirmed absence)
// must never fall through to the local branch — spec "Failure modes"
// requires an inability to check to surface as an error, and silently
// preferring a possibly-stale local branch over an unchecked remote-
// tracking ref could produce a wrong-direction answer on the write-once
// ancestry observation (Review round 1, finding #4).
func resolveDefaultBranchRef(ctx context.Context, checkoutPath, defaultBranch string) (string, error) {
	remote := "refs/remotes/origin/" + defaultBranch
	remoteExists, err := refExists(ctx, checkoutPath, remote)
	if err != nil {
		return "", err
	}
	if remoteExists {
		return remote, nil
	}
	localExists, err := refExists(ctx, checkoutPath, defaultBranch)
	if err != nil {
		return "", err
	}
	if localExists {
		return defaultBranch, nil
	}
	return "", errors.New("neither the remote-tracking ref nor the local default branch resolves")
}

// refExists runs `git rev-parse --verify --quiet <ref>^{commit}` and
// interprets its exit code the same way isAncestor does: 0 = exists,
// 1 = genuinely absent (a routine negative, not an error), anything else
// (acquisition failure, timeout, or another git error) is a failure to
// check and is returned as an error rather than folded into `false`.
func refExists(ctx context.Context, checkoutPath, ref string) (bool, error) {
	runErr, acquireErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, AncestryCallTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
			cmd.Dir = checkoutPath
			return cmd
		})
	if acquireErr != nil {
		return false, acquireErr
	}
	if runErr == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, runErr
}

// isAncestor runs `git merge-base --is-ancestor -- commit ref` and
// interprets its exit code: 0 = true, 1 = false (a routine negative
// result, not an error — see spec "Evidence", squash-merge finding 2),
// anything else is an ancestry error. The `--` separator (unlike
// refExists's rev-parse call, which must NOT take one: it changes
// rev-parse's <ref>^{commit} argument from a revision expression to a
// pathspec and breaks resolution entirely) forces both positional
// arguments to be parsed as revisions, never as options, regardless of
// content — commit is already checked against commitSHAPattern by the
// time it reaches here, but this is the same caller-validation contract
// apps/backend/internal/common/subproc/git_command.go documents, applied
// at the seam that actually shells out (Review round 1, finding #7).
func isAncestor(ctx context.Context, checkoutPath, commit, ref string) (bool, error) {
	runErr, acquireErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, AncestryCallTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "merge-base", "--is-ancestor", "--", commit, ref)
			cmd.Dir = checkoutPath
			return cmd
		})
	if acquireErr != nil {
		return false, acquireErr
	}
	if runErr == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, runErr
}
