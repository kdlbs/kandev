// Package gitbootstrap creates and validates Kandev's local baseline for an
// authenticated remote that has no refs yet.
package gitbootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	// EmptyTreeSHA is the empty tree object in the default SHA-1 Git object
	// format. Repositories using another object format still receive the
	// repository's own empty-tree object from git mktree.
	EmptyTreeSHA  = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	markerPrefix  = "refs/kandev/empty-remote/"
	commitMessage = "Kandev empty remote baseline"
	identityName  = "Kandev"
	identityEmail = "kandev@localhost"
	commitDate    = "1970-01-01T00:00:00Z"
)

var (
	ErrInvalidBaseBranch = errors.New("invalid empty-remote base branch")
	ErrBaselineConflict  = errors.New("empty-remote baseline conflicts with local Git history")
)

// Baseline identifies the local commit and refs that represent an empty
// remote. The marker and base refs must always point at the same commit.
type Baseline struct {
	Commit    string
	BaseRef   string
	MarkerRef string
}

// MarkerRef returns the local marker ref for a validated base branch.
func MarkerRef(baseBranch string) (string, error) {
	branch, err := normalizeBranch(baseBranch)
	if err != nil {
		return "", err
	}
	return markerPrefix + branch, nil
}

// Ensure creates the deterministic local baseline when neither local ref
// exists. It never writes files or contacts the remote. A concurrent local
// history write is reported as a conflict and is never overwritten.
func Ensure(ctx context.Context, repoPath, baseBranch string) (Baseline, error) {
	baseRef, markerRef, err := refsForBranch(baseBranch)
	if err != nil {
		return Baseline{}, err
	}
	existing, present, err := readBaseline(ctx, repoPath, baseRef, markerRef)
	if err != nil {
		return Baseline{}, err
	}
	if present {
		return existing, nil
	}
	if basePresent, err := refPresent(ctx, repoPath, baseRef); err != nil {
		return Baseline{}, err
	} else if basePresent {
		return Baseline{}, ErrBaselineConflict
	}

	tree, err := createEmptyTree(ctx, repoPath)
	if err != nil {
		return Baseline{}, fmt.Errorf("create empty Git tree: %w", err)
	}
	commit, err := createCommit(ctx, repoPath, tree)
	if err != nil {
		return Baseline{}, fmt.Errorf("create empty-remote baseline commit: %w", err)
	}
	baseline := Baseline{Commit: commit, BaseRef: baseRef, MarkerRef: markerRef}
	if err := createRefs(ctx, repoPath, baseline); err != nil {
		verified, verifiedPresent, verifyErr := readBaseline(ctx, repoPath, baseRef, markerRef)
		if verifyErr == nil && verifiedPresent && verified.Commit == commit {
			return verified, nil
		}
		if verifyErr != nil {
			return Baseline{}, fmt.Errorf("publish local empty-remote refs: %w; verify race: %v", err, verifyErr)
		}
		if verifiedPresent {
			return Baseline{}, ErrBaselineConflict
		}
		return Baseline{}, fmt.Errorf("publish local empty-remote refs: %w", err)
	}
	return baseline, nil
}

// Validate reports whether the exact local baseline marker exists. A missing
// marker is not an error because it means this repository is not bootstrapped.
func Validate(ctx context.Context, repoPath, baseBranch string) (Baseline, bool, error) {
	baseRef, markerRef, err := refsForBranch(baseBranch)
	if err != nil {
		return Baseline{}, false, err
	}
	baseline, present, err := readBaseline(ctx, repoPath, baseRef, markerRef)
	return baseline, present, err
}

// Retire removes the local bootstrap marker after the baseline has been
// published. The expected commit protects a concurrent local ref change.
func Retire(ctx context.Context, repoPath string, baseline Baseline) error {
	if strings.TrimSpace(baseline.MarkerRef) == "" || strings.TrimSpace(baseline.Commit) == "" {
		return errors.New("invalid empty-remote baseline marker")
	}
	input := strings.Join([]string{
		"start",
		"delete " + baseline.MarkerRef + " " + baseline.Commit,
		"prepare",
		"commit",
		"",
	}, "\n")
	if _, err := runGitWithInput(ctx, repoPath, input, "update-ref", "--stdin"); err != nil {
		return fmt.Errorf("retire empty-remote baseline marker: %w", err)
	}
	return nil
}

func refsForBranch(baseBranch string) (string, string, error) {
	branch, err := normalizeBranch(baseBranch)
	if err != nil {
		return "", "", err
	}
	return "refs/heads/" + branch, markerPrefix + branch, nil
}

func normalizeBranch(baseBranch string) (string, error) {
	branch := strings.TrimPrefix(strings.TrimSpace(baseBranch), "origin/")
	if !securityutil.IsValidBaseBranchRef(branch) {
		return "", fmt.Errorf("%w: %q", ErrInvalidBaseBranch, baseBranch)
	}
	return branch, nil
}

func readBaseline(ctx context.Context, repoPath, baseRef, markerRef string) (Baseline, bool, error) {
	baseCommit, basePresent, err := readRef(ctx, repoPath, baseRef)
	if err != nil {
		return Baseline{}, false, err
	}
	markerCommit, markerPresent, err := readRef(ctx, repoPath, markerRef)
	if err != nil {
		return Baseline{}, false, err
	}
	if !markerPresent {
		return Baseline{}, false, nil
	}
	if !basePresent || !markerPresent || baseCommit != markerCommit {
		return Baseline{}, false, ErrBaselineConflict
	}
	return Baseline{Commit: baseCommit, BaseRef: baseRef, MarkerRef: markerRef}, true, nil
}

func refPresent(ctx context.Context, repoPath, ref string) (bool, error) {
	_, present, err := readRef(ctx, repoPath, ref)
	return present, err
}

func readRef(ctx context.Context, repoPath, ref string) (string, bool, error) {
	output, err := runGit(ctx, repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if strings.TrimSpace(output) == "" {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read local Git ref %q: %w", ref, err)
	}
	commit := strings.TrimSpace(output)
	if commit == "" {
		return "", false, nil
	}
	return commit, true, nil
}

func createEmptyTree(ctx context.Context, repoPath string) (string, error) {
	output, err := runGitWithInput(ctx, repoPath, "", "mktree")
	if err != nil {
		return "", err
	}
	tree := strings.TrimSpace(output)
	if tree == "" {
		return "", errors.New("git mktree returned no object ID")
	}
	return tree, nil
}

func createCommit(ctx context.Context, repoPath, tree string) (string, error) {
	cmd := subproc.NewGitCommand(ctx,
		"-C", repoPath,
		"-c", "user.name="+identityName,
		"-c", "user.email="+identityEmail,
		"-c", "commit.gpgSign=false",
		"commit-tree", tree, "-m", commitMessage,
	)
	cmd.Env = baselineGitEnvironment()
	output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd)
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	commit := strings.TrimSpace(string(output))
	if commit == "" {
		return "", errors.New("git commit-tree returned no object ID")
	}
	return commit, nil
}

func baselineGitEnvironment() []string {
	const (
		authorName     = "GIT_AUTHOR_NAME="
		authorEmail    = "GIT_AUTHOR_EMAIL="
		committerName  = "GIT_COMMITTER_NAME="
		committerEmail = "GIT_COMMITTER_EMAIL="
	)
	env := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, authorName) || strings.HasPrefix(entry, authorEmail) ||
			strings.HasPrefix(entry, committerName) || strings.HasPrefix(entry, committerEmail) {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"GIT_AUTHOR_DATE="+commitDate,
		"GIT_COMMITTER_DATE="+commitDate,
		"GIT_AUTHOR_NAME="+identityName,
		"GIT_AUTHOR_EMAIL="+identityEmail,
		"GIT_COMMITTER_NAME="+identityName,
		"GIT_COMMITTER_EMAIL="+identityEmail,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func createRefs(ctx context.Context, repoPath string, baseline Baseline) error {
	input := strings.Join([]string{
		"start",
		"create " + baseline.BaseRef + " " + baseline.Commit,
		"create " + baseline.MarkerRef + " " + baseline.Commit,
		"prepare",
		"commit",
		"",
	}, "\n")
	_, err := runGitWithInput(ctx, repoPath, input, "update-ref", "--stdin")
	return err
}

func runGitWithInput(ctx context.Context, repoPath, input string, args ...string) (string, error) {
	cmd := subproc.NewGitCommand(ctx, append([]string{"-C", repoPath}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd)
	if err != nil {
		return string(output), fmt.Errorf("git %v: %s: %w", args, strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	return runGitWithInput(ctx, repoPath, "", args...)
}
