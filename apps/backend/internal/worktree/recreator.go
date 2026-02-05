package worktree

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// RecreateRequest contains parameters for recreating a worktree.
type RecreateRequest struct {
	SessionID    string
	TaskID       string
	TaskTitle    string
	RepositoryID string
	BaseBranch   string
	WorktreeID   string
}

// RecreateResult contains the result of worktree recreation.
type RecreateResult struct {
	WorktreePath   string
	WorktreeBranch string
}

// Recreator provides worktree recreation capability for the orchestrator.
// It wraps the Manager and exposes a simplified interface for recreating
// worktrees when the directory is missing but the database record exists.
type Recreator struct {
	manager *Manager
}

// NewRecreator creates a new Recreator that wraps the given Manager.
func NewRecreator(mgr *Manager) *Recreator {
	return &Recreator{manager: mgr}
}

// Recreate attempts to recreate a worktree when its directory is missing.
// It looks up the existing worktree record by ID or session ID to get the
// stored repository path, then uses the Manager's Create method which
// handles recreation when a worktree record exists but directory is invalid.
func (r *Recreator) Recreate(ctx context.Context, req RecreateRequest) (*RecreateResult, error) {
	if r.manager == nil {
		return nil, fmt.Errorf("worktree manager not available")
	}

	// Look up existing worktree to get stored repository path
	var existingWt *Worktree
	var err error

	// Try by worktree ID first
	if req.WorktreeID != "" {
		existingWt, err = r.manager.GetByID(ctx, req.WorktreeID)
		if err != nil && err != ErrWorktreeNotFound {
			r.manager.logger.Debug("failed to get worktree by ID",
				zap.String("worktree_id", req.WorktreeID),
				zap.Error(err))
		}
	}

	// Try by session ID if not found
	if existingWt == nil && req.SessionID != "" {
		existingWt, err = r.manager.GetBySessionID(ctx, req.SessionID)
		if err != nil && err != ErrWorktreeNotFound {
			r.manager.logger.Debug("failed to get worktree by session ID",
				zap.String("session_id", req.SessionID),
				zap.Error(err))
		}
	}

	if existingWt == nil {
		return nil, fmt.Errorf("worktree record not found for recreation")
	}

	repoPath := existingWt.RepositoryPath
	baseBranch := req.BaseBranch
	if baseBranch == "" {
		baseBranch = existingWt.BaseBranch
	}

	r.manager.logger.Debug("found existing worktree record for recreation",
		zap.String("worktree_id", existingWt.ID),
		zap.String("repository_path", repoPath),
		zap.String("base_branch", baseBranch))

	if repoPath == "" {
		return nil, fmt.Errorf("repository path not found in existing worktree record")
	}

	// Use the manager's Create method which handles recreation internally
	// when a worktree record exists but the directory is invalid (manager.go:167-173)
	createReq := CreateRequest{
		SessionID:      req.SessionID,
		TaskID:         req.TaskID,
		TaskTitle:      req.TaskTitle,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: repoPath,
		BaseBranch:     baseBranch,
		WorktreeID:     req.WorktreeID,
	}

	wt, err := r.manager.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to recreate worktree: %w", err)
	}

	return &RecreateResult{
		WorktreePath:   wt.Path,
		WorktreeBranch: wt.Branch,
	}, nil
}
