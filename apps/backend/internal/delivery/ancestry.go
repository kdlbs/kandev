package delivery

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

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
// <default_branch> when the remote-tracking ref does not exist.
func resolveDefaultBranchRef(ctx context.Context, checkoutPath, defaultBranch string) (string, error) {
	remote := "refs/remotes/origin/" + defaultBranch
	if refExists(ctx, checkoutPath, remote) {
		return remote, nil
	}
	if refExists(ctx, checkoutPath, defaultBranch) {
		return defaultBranch, nil
	}
	return "", errors.New("neither the remote-tracking ref nor the local default branch resolves")
}

func refExists(ctx context.Context, checkoutPath, ref string) bool {
	runErr, acquireErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, AncestryCallTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
			cmd.Dir = checkoutPath
			return cmd
		})
	return acquireErr == nil && runErr == nil
}

// isAncestor runs `git merge-base --is-ancestor commit ref` and interprets
// its exit code: 0 = true, 1 = false (a routine negative result, not an
// error — see spec "Evidence", squash-merge finding 2), anything else is
// an ancestry error.
func isAncestor(ctx context.Context, checkoutPath, commit, ref string) (bool, error) {
	runErr, acquireErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, AncestryCallTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "merge-base", "--is-ancestor", commit, ref)
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
