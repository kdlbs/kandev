package worktree

import (
	"context"
	"fmt"
	"os"

	"github.com/kandev/kandev/internal/common/gitref"
)

type CheckoutIdentity struct {
	WorktreeID string
	PathHash   string
}

// InspectCheckoutIdentity proves the expected branch and reciprocal Git
// registration without reading checkout content or refreshing the index.
func InspectCheckoutIdentity(ctx context.Context, req PreservationRequest) (*CheckoutIdentity, error) {
	path, _, err := inspectCheckoutIdentity(ctx, req)
	if err != nil {
		return nil, err
	}
	return &CheckoutIdentity{WorktreeID: req.WorktreeID, PathHash: hashBytes([]byte(path))}, nil
}

func inspectCheckoutIdentity(ctx context.Context, req PreservationRequest) (string, *preservedGitIdentity, error) {
	repositoryPath, err := canonicalDirectory(req.RepositoryPath)
	if err != nil {
		return "", nil, fmt.Errorf("%w: repository path", ErrPreservedCheckoutUnproven)
	}
	worktreePath, err := canonicalDirectory(req.WorktreePath)
	if err != nil || req.ExpectedBranch == "" || req.WorktreeID == "" {
		return "", nil, fmt.Errorf("%w: checkout path", ErrPreservedCheckoutUnproven)
	}
	identity, err := inspectPreservedGitIdentity(ctx, repositoryPath, worktreePath, req.ExpectedBranch)
	return worktreePath, identity, err
}

// checkoutIdentityMatches reports whether the checkout at checkoutPath and the
// checkout or repository at otherPath share a Git common directory. resolved is
// false when either common directory cannot be resolved, so a caller can fail
// closed instead of treating an unavailable comparison as a mismatch.
func checkoutIdentityMatches(checkoutPath, otherPath string) (matches, resolved bool) {
	if checkoutPath == "" || otherPath == "" {
		return false, false
	}
	checkoutCommonDir, err := resolveCommonGitDirPath(checkoutPath)
	if err != nil {
		return false, false
	}
	otherCommonDir, err := resolveCommonGitDirPath(otherPath)
	if err != nil {
		return false, false
	}
	checkoutInfo, err := os.Stat(checkoutCommonDir)
	if err != nil || !checkoutInfo.IsDir() {
		return false, false
	}
	otherInfo, err := os.Stat(otherCommonDir)
	if err != nil || !otherInfo.IsDir() {
		return false, false
	}
	return os.SameFile(checkoutInfo, otherInfo), true
}

// resolveCommonGitDirPath resolves the shared Git directory of a checkout
// without invoking Git. For a linked worktree the .git pointer file is
// followed to its admin directory and the admin's commondir file yields the
// shared directory; for a regular repository it is the checkout's own .git.
func resolveCommonGitDirPath(path string) (string, error) {
	gitDir, err := gitref.ResolveGitDir(path)
	if err != nil {
		return "", err
	}
	return gitref.ResolveCommonGitDir(gitDir), nil
}
