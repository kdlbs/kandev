package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type worktreeAdoptionCleanupProvider interface {
	CaptureWorkspaceAdoptionsForCleanup(context.Context, []*worktree.Worktree) (map[string]*models.WorkspaceAdoptionReceipt, error)
}

func (s *Service) captureCleanupAdoptions(ctx context.Context, worktrees []*worktree.Worktree) (map[string]*models.WorkspaceAdoptionReceipt, error) {
	provider, ok := s.worktreeCleanup.(worktreeAdoptionCleanupProvider)
	if !ok {
		return nil, nil
	}
	receipts, err := provider.CaptureWorkspaceAdoptionsForCleanup(ctx, worktrees)
	if err != nil {
		return nil, fmt.Errorf("capture cleanup adoption provenance: %w", err)
	}
	return receipts, nil
}
