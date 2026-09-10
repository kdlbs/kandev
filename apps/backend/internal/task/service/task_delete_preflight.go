package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/worktree"
)

// ErrTaskDeletePreflightInvalid identifies a malformed delete selection.
var ErrTaskDeletePreflightInvalid = errors.New("invalid task delete preflight")

// ErrTaskDeletePreflightUnavailable identifies an inspection dependency or
// read failure. Callers must not interpret this error as a clean workspace.
var ErrTaskDeletePreflightUnavailable = errors.New("task delete preflight unavailable")

// TaskDeletePreflightResult contains the current local-change requirement for
// the exact task scope requested by the caller.
type TaskDeletePreflightResult struct {
	RequiresDiscardConsent bool `json:"requires_discard_consent"`
}

// TaskDeletePreflight inspects all worktrees that a delete would remove. It is
// read-only and intentionally does not prepare cleanup, stop execution, or
// mutate task rows. DeleteTaskWithOptions remains the authoritative recheck.
func (s *Service) TaskDeletePreflight(
	ctx context.Context, taskIDs []string, cascade bool,
) (TaskDeletePreflightResult, error) {
	roots := normalizeTaskDeletePreflightIDs(taskIDs)
	if len(roots) == 0 {
		return TaskDeletePreflightResult{}, fmt.Errorf("%w: task_ids is required", ErrTaskDeletePreflightInvalid)
	}
	if s.tasks == nil {
		return TaskDeletePreflightResult{}, fmt.Errorf("%w: task repository is not configured", ErrTaskDeletePreflightUnavailable)
	}
	targets, err := s.resolveTaskDeletePreflightTargets(ctx, roots, cascade)
	if err != nil {
		return TaskDeletePreflightResult{}, err
	}
	provider, inspector, err := s.taskDeletePreflightInspector()
	if err != nil {
		return TaskDeletePreflightResult{}, err
	}
	worktrees := make([]*worktree.Worktree, 0)
	for _, taskID := range targets {
		inventory, inventoryErr := provider.GetAllByTaskID(ctx, taskID)
		if inventoryErr != nil {
			return TaskDeletePreflightResult{}, fmt.Errorf(
				"%w: list worktrees for %s: %v", ErrTaskDeletePreflightUnavailable, taskID, inventoryErr,
			)
		}
		worktrees = append(worktrees, inventory...)
	}
	dirty, err := inspector.InspectDirtyWorktrees(ctx, worktrees)
	if err != nil {
		return TaskDeletePreflightResult{}, fmt.Errorf(
			"%w: inspect worktrees before delete: %v", ErrTaskDeletePreflightUnavailable, err,
		)
	}
	return TaskDeletePreflightResult{RequiresDiscardConsent: len(dirty) > 0}, nil
}

func normalizeTaskDeletePreflightIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	ids := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		ids = append(ids, taskID)
	}
	return ids
}

func (s *Service) taskDeletePreflightInspector() (WorktreeProvider, WorktreeDirtyInspector, error) {
	if s.worktreeCleanup == nil {
		return nil, nil, fmt.Errorf("%w: worktree cleanup is not configured", ErrTaskDeletePreflightUnavailable)
	}
	provider, ok := s.worktreeCleanup.(WorktreeProvider)
	if !ok {
		return nil, nil, fmt.Errorf("%w: worktree inventory is not configured", ErrTaskDeletePreflightUnavailable)
	}
	inspector, ok := s.worktreeCleanup.(WorktreeDirtyInspector)
	if !ok {
		return nil, nil, fmt.Errorf("%w: worktree inspection is not configured", ErrTaskDeletePreflightUnavailable)
	}
	return provider, inspector, nil
}

func (s *Service) resolveTaskDeletePreflightTargets(
	ctx context.Context, roots []string, cascade bool,
) ([]string, error) {
	targets := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		taskID := strings.TrimSpace(queue[0])
		queue = queue[1:]
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
			return nil, err
		}
		if _, scoped := callerScope(ctx); !scoped {
			if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
				return nil, err
			}
		}
		targets = append(targets, taskID)
		if !cascade {
			continue
		}
		children, err := s.tasks.ListChildrenIncludingArchived(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("%w: list delete descendants of %s: %v", ErrTaskDeletePreflightUnavailable, taskID, err)
		}
		for _, child := range children {
			if child != nil && strings.TrimSpace(child.ID) != "" {
				queue = append(queue, child.ID)
			}
		}
	}
	return targets, nil
}
