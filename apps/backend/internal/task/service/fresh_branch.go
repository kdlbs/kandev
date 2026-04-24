package service

import (
	"context"
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
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", baseBranch)
	fetchCmd.Dir = absPath
	_ = fetchCmd.Run()

	// Use `-b` (not `-B`) so we refuse to overwrite an existing branch — that
	// would silently orphan commits only reachable from it.
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", newBranch, baseBranch)
	cmd.Dir = absPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %q from %q: %w (%s)", newBranch, baseBranch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sanitizeGitRef returns name unchanged when it matches the allowlist, or an
// error otherwise. The returned value is the trusted ref string callers
// should pass to git.
func sanitizeGitRef(name, label string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if !gitRefRe.MatchString(name) {
		return "", fmt.Errorf("invalid %s name %q", label, name)
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid %s name %q", label, name)
	}
	return name, nil
}

func discardLocalChanges(ctx context.Context, repoPath string) error {
	for _, args := range [][]string{
		{"reset", "--hard"},
		{"clean", "-fd"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
