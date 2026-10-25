package worktree

import (
	"context"
	"fmt"
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
