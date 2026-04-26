package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// gitRefRe accepts the conservative subset of git ref names we expose to user
// input: ASCII letters, digits, and `._/-`, with no leading `-` (which git
// would treat as an option). This rejects any value that could be interpreted
// as a flag by `git fetch` / `git checkout` even though we pass refs as
// positional argv entries.
var gitRefRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_./-]*$`)

// ErrInvalidGitRef is returned by sanitizeGitRef when the supplied ref name
// doesn't match the allowlist. Returned errors wrap this sentinel so the
// HTTP layer can map it to a 400 response while preserving the descriptive
// inner message.
var ErrInvalidGitRef = errors.New("invalid git ref name")

// ErrFreshBranchCheckout is returned when the underlying `git checkout`
// fails (e.g. the requested NewBranch already exists or BaseBranch can't be
// resolved). The caller can map this to a 4xx + the wrapped message.
var ErrFreshBranchCheckout = errors.New("checkout failed")

// ErrPartialDiscard is returned when discarding the working tree partially
// succeeded — `git reset --hard` mutated tracked files but `git clean -fd`
// then failed (locked file, permission error, etc.) so untracked files
// survive. Callers must surface a distinct message: a generic 500 would
// hide the fact that some user work has already been destroyed.
var ErrPartialDiscard = errors.New("partial discard: tracked changes were reset but git clean failed")

// FreshBranchRequest performs a destructive checkout on a local repository:
// discard uncommitted changes, then create NewBranch from BaseBranch.
//
// ConsentedDirtyFiles is the dirty-file list the caller already showed to
// the user. The backend re-reads dirty files at execution time and rejects
// the request (with the new list) if any path appears that wasn't on the
// consented list — this protects against silent loss of files that became
// dirty between the consent dialog and the actual discard.
type FreshBranchRequest struct {
	RepoPath            string
	BaseBranch          string
	NewBranch           string
	ConfirmDiscard      bool
	ConsentedDirtyFiles []string
}

// ErrDirtyWorkingTree is returned by PerformFreshBranch when the repository
// has uncommitted changes and the caller did not set ConfirmDiscard, OR
// when the dirty file set grew beyond the consented list. Callers should
// surface DirtyFiles to the user and re-issue the request with
// ConfirmDiscard=true and ConsentedDirtyFiles=DirtyFiles once consent is
// re-confirmed.
type ErrDirtyWorkingTree struct {
	DirtyFiles []string
}

func (e *ErrDirtyWorkingTree) Error() string {
	return fmt.Sprintf("working tree has %d uncommitted change(s)", len(e.DirtyFiles))
}

// PerformFreshBranch validates the request, optionally discards uncommitted
// changes, then creates the new branch from the base branch.
//
// On success the repository is left checked out on NewBranch. The caller is
// responsible for persisting NewBranch as the task's effective base branch so
// that future session resumes return to it.
func (s *Service) PerformFreshBranch(ctx context.Context, req FreshBranchRequest) error {
	// Validate refs against an allowlist before we shell out. The returned
	// values are the trusted strings used for the rest of the function — we
	// never use req.NewBranch / req.BaseBranch directly past this point.
	newBranch, err := sanitizeGitRef(req.NewBranch, "new branch")
	if err != nil {
		return err
	}
	baseBranch, err := sanitizeGitRef(req.BaseBranch, "base branch")
	if err != nil {
		return err
	}
	absPath, err := s.resolveAllowedLocalPath(req.RepoPath)
	if err != nil {
		return err
	}

	dirty, err := readGitDirtyFiles(ctx, absPath)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		if !req.ConfirmDiscard {
			return &ErrDirtyWorkingTree{DirtyFiles: dirty}
		}
		consented := make(map[string]struct{}, len(req.ConsentedDirtyFiles))
		for _, p := range req.ConsentedDirtyFiles {
			consented[p] = struct{}{}
		}
		for _, p := range dirty {
			if _, ok := consented[p]; !ok {
				return &ErrDirtyWorkingTree{DirtyFiles: dirty}
			}
		}
		if err := discardLocalChanges(ctx, absPath); err != nil {
			return err
		}
	}

	// Best-effort fetch so a remote-tracking ref like "origin/main" resolves.
	_, _ = runGit(ctx, absPath, "fetch", "origin", baseBranch)

	// Use `-b` (not `-B`) so we refuse to overwrite an existing branch — that
	// would silently orphan commits only reachable from it.
	if out, err := runGit(ctx, absPath, "checkout", "-b", newBranch, baseBranch); err != nil {
		return fmt.Errorf("%w: %q from %q: %s", ErrFreshBranchCheckout, newBranch, baseBranch, out)
	}
	return nil
}

// sanitizeGitRef returns name unchanged when it matches the allowlist, or an
// error otherwise. The returned value is the trusted ref string callers
// should pass to git.
func sanitizeGitRef(name, label string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidGitRef, label)
	}
	if !gitRefRe.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: %s %q", ErrInvalidGitRef, label, name)
	}
	return name, nil
}

func discardLocalChanges(ctx context.Context, repoPath string) error {
	if out, err := runGit(ctx, repoPath, "reset", "--hard"); err != nil {
		return fmt.Errorf("git reset --hard: %w (%s)", err, out)
	}
	if out, err := runGit(ctx, repoPath, "clean", "-fd"); err != nil {
		// reset already mutated tracked files; signal partial state so the
		// HTTP handler can surface a distinct message instead of a generic 500.
		return fmt.Errorf("%w: git clean -fd: %v (%s)", ErrPartialDiscard, err, out)
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
