package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// BlockerRepository provides access to task blocker persistence.
type BlockerRepository interface {
	CreateTaskBlocker(ctx context.Context, blocker *orchmodels.TaskBlocker) error
	ListTaskBlockers(ctx context.Context, taskID string) ([]*orchmodels.TaskBlocker, error)
	DeleteTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error
	// ListTasksBlockedBy returns task IDs that have blockerTaskID listed as
	// one of their blockers (the reverse direction of ListTaskBlockers).
	ListTasksBlockedBy(ctx context.Context, blockerTaskID string) ([]string, error)
	// ListBlockersForTasks batches the forward direction over many tasks.
	ListBlockersForTasks(ctx context.Context, taskIDs []string) (map[string][]string, error)
	// ListDependentsForTasks batches the reverse direction over many tasks.
	ListDependentsForTasks(ctx context.Context, blockerTaskIDs []string) (map[string][]string, error)
}

// taskDependencyCleaner removes every edge touching a task. Optional on
// BlockerRepository so existing test doubles keep compiling.
type taskDependencyCleaner interface {
	DeleteTaskBlockersForTask(ctx context.Context, taskID string) error
}

// CommentRepository provides access to task comment persistence.
type CommentRepository interface {
	CreateTaskComment(ctx context.Context, comment *orchmodels.TaskComment) error
	ListTaskComments(ctx context.Context, taskID string) ([]*orchmodels.TaskComment, error)
}

// TaskStateActivityLogger records a durable status transition before the task
// state event is published. Optional callers can use this seam to keep
// read-model activity and WebSocket notifications ordered.
type TaskStateActivityLogger interface {
	LogTaskStateChange(ctx context.Context, task *models.Task, oldState v1.TaskState)
}

// SetBlockerRepository wires the blocker repository for office integration.
func (s *Service) SetBlockerRepository(repo BlockerRepository) {
	s.blockers = repo
}

// SetCommentRepository wires the comment repository for office integration.
func (s *Service) SetCommentRepository(repo CommentRepository) {
	s.comments = repo
}

// SetTaskStateActivityLogger wires the optional durable activity writer used
// before task.state_changed notifications are published.
func (s *Service) SetTaskStateActivityLogger(logger TaskStateActivityLogger) {
	s.taskStateActivity = logger
}

// GetLastAgentMessage returns the content of the most recent agent message
// in a session. Used by the office comment bridge to auto-post agent responses.
func (s *Service) GetLastAgentMessage(ctx context.Context, sessionID string) (string, error) {
	return s.sessions.GetLastAgentMessage(ctx, sessionID)
}

// GetLastAgentMessageForTurn returns the most recent text agent message for a
// single turn. Used by the office comment bridge when a late terminal complete
// refers to a historical turn while a newer turn may already be active.
func (s *Service) GetLastAgentMessageForTurn(ctx context.Context, turnID string) (string, error) {
	messages, err := s.messages.ListMessagesByTurnID(ctx, turnID)
	if err != nil {
		return "", err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message == nil ||
			message.AuthorType != models.MessageAuthorAgent ||
			message.Type != models.MessageTypeMessage ||
			message.Content == "" {
			continue
		}
		return message.Content, nil
	}
	return "", nil
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
//
// Thin alias for AddDependency, which owns the only edge validator (self-edge,
// cross-workspace, BFS cycle with a reportable path). This used to run its own
// weaker check; two validators meant a cycle could enter through whichever path
// was laxer.
func (s *Service) AddBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	return s.AddDependency(ctx, taskID, blockerTaskID)
}

// RemoveBlocker removes a blocker relationship between two tasks.
func (s *Service) RemoveBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	return s.RemoveDependency(ctx, taskID, blockerTaskID)
}

// createBlockerEdge writes the dependency row.
//
// The row type is Office-owned, and ARCH-TASK-OFFICE-IMPORT keeps new task-tier
// files free of Office imports; this file is already baselined for it, so the
// construction is confined here rather than widening the baseline.
func (s *Service) createBlockerEdge(ctx context.Context, taskID, blockerTaskID string) error {
	return s.blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{
		TaskID:        taskID,
		BlockerTaskID: blockerTaskID,
	})
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
// Like GetBlockers, it returns the raw blocker edges: archived tasks are not
// filtered out, so callers that only want active tasks must filter themselves.
func (s *Service) GetBlocking(ctx context.Context, taskID string) ([]string, error) {
	if s.blockers == nil {
		return nil, fmt.Errorf("blocker repository not configured")
	}
	ids, err := s.blockers.ListTasksBlockedBy(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		return []string{}, nil
	}
	return ids, nil
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
