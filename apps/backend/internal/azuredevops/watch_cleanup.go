package azuredevops

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// TaskSessionChecker reports whether a user interacted with a task beyond
// the watch's automatic start message. It is intentionally tiny so Azure can
// reuse the shared backend adapter without importing task repository types.
type TaskSessionChecker interface {
	HasUserAuthoredMessage(context.Context, string) (bool, error)
}

func (s *Service) CleanupWorkItemWatch(ctx context.Context, workspaceID, watchID string) (int, error) {
	watch, err := s.authorizedWorkItemWatch(ctx, workspaceID, watchID)
	if err != nil {
		return 0, err
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return s.cleanupWorkItemWatchWithClient(ctx, watch, client), nil
}

func (s *Service) CleanupPullRequestWatch(ctx context.Context, workspaceID, watchID string) (int, error) {
	watch, err := s.authorizedPullRequestWatch(ctx, workspaceID, watchID)
	if err != nil {
		return 0, err
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return s.cleanupPullRequestWatchWithClient(ctx, watch, client), nil
}

func (s *Service) cleanupWorkItemWatchWithClient(ctx context.Context, watch *WorkItemWatch, client Client) int {
	if watch == nil || client == nil || watch.CleanupPolicy == CleanupPolicyNever {
		return 0
	}
	rows, err := s.store.ListWorkItemWatchTasks(ctx, watch.ID, watch.Generation)
	if err != nil {
		s.log.Warn("azure devops watcher: list work-item cleanup rows", zap.String("watch_id", watch.ID), zap.Error(err))
		return 0
	}
	deleted := 0
	for _, row := range rows {
		if row == nil || row.TaskID == "" {
			continue
		}
		item, fetchErr := client.GetWorkItem(ctx, watch.ProjectID, row.WorkItemID)
		if fetchErr != nil || item == nil || !isTerminalWorkItemState(item.State) {
			continue
		}
		if !s.cleanupTaskAllowed(ctx, row.TaskID, watch.CleanupPolicy) {
			continue
		}
		if s.deleteWatchTaskTree(ctx, watch.ID, row.TaskID) {
			if err := s.store.DeleteWorkItemWatchTask(ctx, row.ID); err != nil {
				s.log.Warn("azure devops watcher: remove work-item cleanup row", zap.String("watch_id", watch.ID), zap.String("row_id", row.ID), zap.Error(err))
				continue
			}
			deleted++
		}
	}
	return deleted
}

func (s *Service) cleanupPullRequestWatchWithClient(ctx context.Context, watch *PullRequestWatch, client Client) int {
	if watch == nil || client == nil || watch.CleanupPolicy == CleanupPolicyNever {
		return 0
	}
	rows, err := s.store.ListPullRequestWatchTasks(ctx, watch.ID, watch.Generation)
	if err != nil {
		s.log.Warn("azure devops watcher: list pull-request cleanup rows", zap.String("watch_id", watch.ID), zap.Error(err))
		return 0
	}
	deleted := 0
	for _, row := range rows {
		if row == nil || row.TaskID == "" {
			continue
		}
		pullRequest, fetchErr := client.GetPullRequest(ctx, watch.ProjectID, row.AzureRepositoryID, row.PullRequestID)
		if fetchErr != nil || pullRequest == nil || !isTerminalPullRequestStatus(pullRequest.Status) {
			continue
		}
		if !s.cleanupTaskAllowed(ctx, row.TaskID, watch.CleanupPolicy) {
			continue
		}
		if s.deleteWatchTaskTree(ctx, watch.ID, row.TaskID) {
			if err := s.store.DeletePullRequestWatchTask(ctx, row.ID); err != nil {
				s.log.Warn("azure devops watcher: remove pull-request cleanup row", zap.String("watch_id", watch.ID), zap.String("row_id", row.ID), zap.Error(err))
				continue
			}
			deleted++
		}
	}
	return deleted
}

func (s *Service) cleanupTaskAllowed(ctx context.Context, taskID, policy string) bool {
	if policy != CleanupPolicyAuto {
		return true
	}
	checker := s.getTaskSessionChecker()
	if checker == nil {
		return true
	}
	authored, err := checker.HasUserAuthoredMessage(ctx, taskID)
	if err != nil {
		s.log.Warn("azure devops watcher: check task engagement", zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return !authored
}

func (s *Service) deleteWatchTaskTree(ctx context.Context, watchID, taskID string) bool {
	deleter := s.getCascadeTaskDeleter()
	if deleter == nil {
		return false
	}
	if _, err := deleter.DeleteTaskTree(ctx, taskID, true); err != nil {
		s.log.Warn("azure devops watcher: terminal task cleanup failed", zap.String("watch_id", watchID), zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return true
}

func isTerminalWorkItemState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "closed", "done", "completed", "resolved", "removed":
		return true
	default:
		return false
	}
}

func isTerminalPullRequestStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "abandoned", "closed", "merged":
		return true
	default:
		return false
	}
}
