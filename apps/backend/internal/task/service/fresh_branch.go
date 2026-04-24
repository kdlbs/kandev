package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// FreshBranchRequest performs a destructive checkout on a local repository:
// discard uncommitted changes, then create NewBranch from BaseBranch.
type FreshBranchRequest struct {
	RepoPath       string
	BaseBranch     string
	NewBranch      string
	ConfirmDiscard bool
}

// ErrDirtyWorkingTree is returned by PerformFreshBranch when the repository
// has uncommitted changes and the caller did not set ConfirmDiscard. Callers
// should surface DirtyFiles to the user and re-issue the request with
// ConfirmDiscard=true once consent is given.
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
	if req.NewBranch == "" {
		return fmt.Errorf("new branch name is required")
	}
	if req.BaseBranch == "" {
		return fmt.Errorf("base branch is required")
	}
	absPath, err := s.resolveAllowedLocalPath(req.RepoPath)
	if err != nil {
		return err
	}

	dirty, err := readGitDirtyFiles(ctx, absPath)
	if err != nil {
		return err
	}
	if len(dirty) > 0 && !req.ConfirmDiscard {
		return &ErrDirtyWorkingTree{DirtyFiles: dirty}
	}
	if len(dirty) > 0 {
		if err := discardLocalChanges(ctx, absPath); err != nil {
			return err
		}
	}

	// Best-effort fetch so a remote-tracking ref like "origin/main" resolves.
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", req.BaseBranch)
	fetchCmd.Dir = absPath
	_ = fetchCmd.Run()

	cmd := exec.CommandContext(ctx, "git", "checkout", "-B", req.NewBranch, req.BaseBranch)
	cmd.Dir = absPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %q from %q: %w (%s)", req.NewBranch, req.BaseBranch, err, strings.TrimSpace(string(out)))
	}
	return nil
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
