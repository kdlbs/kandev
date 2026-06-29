package gitlab

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// CleanupAllReviewTasks deletes auto-created review tasks for merged/closed MRs,
// respecting each watch's cleanup policy. Returns the count of tasks deleted.
func (s *Service) CleanupAllReviewTasks(ctx context.Context) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.taskDeleter
	checker := s.taskSessionChecker
	s.mu.RUnlock()
	if deleter == nil {
		return 0, errors.New("task deleter not configured")
	}
	tasks, err := store.ListAllReviewMRTasks(ctx)
	if err != nil {
		return 0, err
	}
	return s.sweepReviewMRTasks(ctx, tasks, deleter, checker), nil
}

// deleteTaskWithReason deletes a task, threading the cleanup reason through to
// the task.deleted event when the wired deleter supports it (see
// TaskDeleterWithReason). Falls back to plain DeleteTask otherwise.
func deleteTaskWithReason(ctx context.Context, deleter TaskDeleter, taskID, reason string) error {
	if d, ok := deleter.(TaskDeleterWithReason); ok {
		return d.DeleteTaskWithReason(ctx, taskID, reason)
	}
	return deleter.DeleteTask(ctx, taskID)
}

func (s *Service) sweepReviewMRTasks(ctx context.Context, tasks []*ReviewMRTask, deleter TaskDeleter, checker TaskSessionChecker) int {
	client := s.Client()
	if client == nil {
		return 0
	}
	policies := s.resolveReviewWatchPolicies(ctx, tasks)
	deleted := 0
	for _, t := range tasks {
		if s.deleteReviewMRTaskIfTerminal(ctx, t, policies[t.ReviewWatchID], client, deleter, checker) {
			deleted++
		}
	}
	return deleted
}

func (s *Service) resolveReviewWatchPolicies(ctx context.Context, tasks []*ReviewMRTask) map[string]string {
	policies := map[string]string{}
	for _, t := range tasks {
		if _, ok := policies[t.ReviewWatchID]; ok {
			continue
		}
		policies[t.ReviewWatchID] = s.lookupReviewPolicy(ctx, t.ReviewWatchID)
	}
	return policies
}

func (s *Service) lookupReviewPolicy(ctx context.Context, watchID string) string {
	store := s.requireStore()
	if store == nil {
		return CleanupPolicyNever
	}
	w, err := store.GetReviewWatch(ctx, watchID)
	if err == nil && w != nil {
		return w.CleanupPolicy
	}
	// On transient DB error or genuinely-missing watch, fall back to
	// CleanupPolicyNever so we never silently delete a task that the user
	// might still care about. The genuine "watch was deleted" case is
	// uncommon enough that requiring the user to manually delete the
	// orphan tasks is the safer trade-off.
	if err != nil {
		s.logger.Warn("lookup review watch policy", zap.String("watch_id", watchID), zap.Error(err))
	}
	return CleanupPolicyNever
}

func (s *Service) deleteReviewMRTaskIfTerminal(ctx context.Context, t *ReviewMRTask, policy string, client Client, deleter TaskDeleter, checker TaskSessionChecker) bool {
	if t.TaskID == "" || policy == CleanupPolicyNever {
		return false
	}
	mrStatus, err := client.GetMRStatus(ctx, t.ProjectPath, t.MRIID)
	if err != nil || mrStatus == nil || mrStatus.MR == nil {
		return false
	}
	state := mrStatus.MR.State
	if state != gitlabStateMerged && state != gitlabStateClosed {
		return false
	}
	if policy == CleanupPolicyAuto && checker != nil {
		// On a transient DB error preserve the task — silently deleting
		// something the user might have engaged with is a much worse
		// outcome than leaving an orphan to be reaped on the next sweep.
		authored, err := checker.HasUserAuthoredMessage(ctx, t.TaskID)
		if err != nil {
			s.logger.Warn("check user authored message during cleanup",
				zap.String("task_id", t.TaskID), zap.Error(err))
			return false
		}
		if authored {
			return false
		}
	}
	if err := deleteTaskWithReason(ctx, deleter, t.TaskID, "pr_merged_or_closed"); err != nil {
		s.logger.Warn("delete review task during cleanup",
			zap.String("task_id", t.TaskID), zap.Error(err))
		return false
	}
	if store := s.requireStore(); store != nil {
		_ = store.DeleteReviewMRTask(ctx, t.ID)
	}
	return true
}

// CleanupAllIssueTasks deletes auto-created issue tasks for closed issues.
func (s *Service) CleanupAllIssueTasks(ctx context.Context) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.taskDeleter
	checker := s.taskSessionChecker
	s.mu.RUnlock()
	if deleter == nil {
		return 0, errors.New("task deleter not configured")
	}
	tasks, err := store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		return 0, err
	}
	return s.sweepIssueWatchTasks(ctx, tasks, deleter, checker), nil
}

func (s *Service) sweepIssueWatchTasks(ctx context.Context, tasks []*IssueWatchTask, deleter TaskDeleter, checker TaskSessionChecker) int {
	client := s.Client()
	if client == nil {
		return 0
	}
	policies := s.resolveIssueWatchPolicies(ctx, tasks)
	deleted := 0
	for _, t := range tasks {
		if s.deleteIssueWatchTaskIfTerminal(ctx, t, policies[t.IssueWatchID], client, deleter, checker) {
			deleted++
		}
	}
	return deleted
}

func (s *Service) resolveIssueWatchPolicies(ctx context.Context, tasks []*IssueWatchTask) map[string]string {
	policies := map[string]string{}
	for _, t := range tasks {
		if _, ok := policies[t.IssueWatchID]; ok {
			continue
		}
		policies[t.IssueWatchID] = s.lookupIssuePolicy(ctx, t.IssueWatchID)
	}
	return policies
}

func (s *Service) lookupIssuePolicy(ctx context.Context, watchID string) string {
	store := s.requireStore()
	if store == nil {
		return CleanupPolicyNever
	}
	w, err := store.GetIssueWatch(ctx, watchID)
	if err == nil && w != nil {
		return w.CleanupPolicy
	}
	if err != nil {
		s.logger.Warn("lookup issue watch policy", zap.String("watch_id", watchID), zap.Error(err))
	}
	return CleanupPolicyNever
}

func (s *Service) deleteIssueWatchTaskIfTerminal(ctx context.Context, t *IssueWatchTask, policy string, client Client, deleter TaskDeleter, checker TaskSessionChecker) bool {
	if t.TaskID == "" || policy == CleanupPolicyNever {
		return false
	}
	state, err := client.GetIssueState(ctx, t.ProjectPath, t.IssueIID)
	if err != nil || state != gitlabStateClosed {
		return false
	}
	if policy == CleanupPolicyAuto && checker != nil {
		authored, err := checker.HasUserAuthoredMessage(ctx, t.TaskID)
		if err != nil {
			s.logger.Warn("check user authored message during cleanup",
				zap.String("task_id", t.TaskID), zap.Error(err))
			return false
		}
		if authored {
			return false
		}
	}
	if err := deleteTaskWithReason(ctx, deleter, t.TaskID, "issue_closed"); err != nil {
		s.logger.Warn("delete issue task during cleanup",
			zap.String("task_id", t.TaskID), zap.Error(err))
		return false
	}
	if store := s.requireStore(); store != nil {
		_ = store.DeleteIssueWatchTask(ctx, t.ID)
	}
	return true
}
