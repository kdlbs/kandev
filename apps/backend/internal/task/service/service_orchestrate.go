package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	orchmodels "github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/task/models"
)

// BlockerRepository provides access to task blocker persistence.
type BlockerRepository interface {
	CreateTaskBlocker(ctx context.Context, blocker *orchmodels.TaskBlocker) error
	ListTaskBlockers(ctx context.Context, taskID string) ([]*orchmodels.TaskBlocker, error)
	DeleteTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error
}

// CommentRepository provides access to task comment persistence.
type CommentRepository interface {
	CreateTaskComment(ctx context.Context, comment *orchmodels.TaskComment) error
	ListTaskComments(ctx context.Context, taskID string) ([]*orchmodels.TaskComment, error)
}

// SetBlockerRepository wires the blocker repository for orchestrate integration.
func (s *Service) SetBlockerRepository(repo BlockerRepository) {
	s.blockers = repo
}

// SetCommentRepository wires the comment repository for orchestrate integration.
func (s *Service) SetCommentRepository(repo CommentRepository) {
	s.comments = repo
}

// ListTaskTree returns a flat list of non-archived tasks for tree building.
func (s *Service) ListTaskTree(ctx context.Context, workspaceID string, filters models.TaskTreeFilters) ([]*models.Task, error) {
	tasks, err := s.tasks.ListTaskTree(ctx, workspaceID, filters)
	if err != nil {
		return nil, err
	}
	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	return tasks, nil
}

// ListTasksByProject returns all tasks for a given project.
func (s *Service) ListTasksByProject(ctx context.Context, projectID string) ([]*models.Task, error) {
	tasks, err := s.tasks.ListTasksByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	return tasks, nil
}

// ListTasksByAssignee returns all tasks assigned to a given agent instance.
func (s *Service) ListTasksByAssignee(ctx context.Context, agentInstanceID string) ([]*models.Task, error) {
	tasks, err := s.tasks.ListTasksByAssignee(ctx, agentInstanceID)
	if err != nil {
		return nil, err
	}
	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	return tasks, nil
}

// AddBlocker creates a blocker relationship between two tasks.
// It validates that the blocker does not create a circular dependency.
func (s *Service) AddBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	if s.blockers == nil {
		return fmt.Errorf("blocker repository not configured")
	}
	if taskID == blockerTaskID {
		return fmt.Errorf("a task cannot block itself")
	}

	// Check for circular dependency: if blockerTaskID is already (transitively) blocked by taskID
	if err := s.checkCircularBlocker(ctx, taskID, blockerTaskID); err != nil {
		return err
	}

	blocker := &orchmodels.TaskBlocker{
		TaskID:        taskID,
		BlockerTaskID: blockerTaskID,
	}
	return s.blockers.CreateTaskBlocker(ctx, blocker)
}

// RemoveBlocker removes a blocker relationship between two tasks.
func (s *Service) RemoveBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	if s.blockers == nil {
		return fmt.Errorf("blocker repository not configured")
	}
	return s.blockers.DeleteTaskBlocker(ctx, taskID, blockerTaskID)
}

// GetBlockers returns all tasks that block the given task.
func (s *Service) GetBlockers(ctx context.Context, taskID string) ([]string, error) {
	if s.blockers == nil {
		return nil, fmt.Errorf("blocker repository not configured")
	}
	blockers, err := s.blockers.ListTaskBlockers(ctx, taskID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(blockers))
	for i, b := range blockers {
		ids[i] = b.BlockerTaskID
	}
	return ids, nil
}

// GetBlocking returns all task IDs that the given task is blocking.
// This is the reverse lookup: find all tasks where blockerTaskID = taskID.
func (s *Service) GetBlocking(ctx context.Context, taskID string) ([]string, error) {
	if s.blockers == nil {
		return nil, fmt.Errorf("blocker repository not configured")
	}
	// We need to search across all tasks. For now, we use a pragmatic approach:
	// query all tasks and check. This will be optimized with a dedicated query later.
	// For the initial implementation, we return an empty list.
	// TODO: Add a GetBlockingTasks query to the blocker repository
	return []string{}, nil
}

// checkCircularBlocker checks if adding blockerTaskID as a blocker to taskID
// would create a circular dependency. It walks the blocker chain from
// blockerTaskID to see if it leads back to taskID.
func (s *Service) checkCircularBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	// Walk the transitive blockers of blockerTaskID. If any path reaches
	// taskID, adding this blocker would create a cycle.
	target := taskID
	visited := make(map[string]bool)
	queue := []string{blockerTaskID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		blockers, err := s.blockers.ListTaskBlockers(ctx, current)
		if err != nil {
			return fmt.Errorf("failed to check circular dependency: %w", err)
		}
		for _, b := range blockers {
			if b.BlockerTaskID == target {
				return fmt.Errorf("circular dependency detected: adding %s as blocker of %s creates a cycle",
					blockerTaskID, taskID)
			}
			if !visited[b.BlockerTaskID] {
				queue = append(queue, b.BlockerTaskID)
			}
		}
	}
	return nil
}

// CreateComment creates a new comment on a task.
func (s *Service) CreateComment(ctx context.Context, comment *orchmodels.TaskComment) error {
	if s.comments == nil {
		return fmt.Errorf("comment repository not configured")
	}
	return s.comments.CreateTaskComment(ctx, comment)
}

// ListComments returns all comments for a task, ordered by creation time.
func (s *Service) ListComments(ctx context.Context, taskID string) ([]*orchmodels.TaskComment, error) {
	if s.comments == nil {
		return nil, fmt.Errorf("comment repository not configured")
	}
	return s.comments.ListTaskComments(ctx, taskID)
}
