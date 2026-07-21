package gitlab

import (
	"context"
	"errors"
	"fmt"
)

// ReserveReviewMRTask atomically claims an MR for task creation. Returns
// (true, nil) when this caller wins the race, (false, nil) when another
// caller has already reserved (or task created).
func (s *Service) ReserveReviewMRTask(ctx context.Context, watchID string, generation int64, projectPath string, iid int, mrURL string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	return store.ReserveReviewMRTaskForGeneration(ctx, watchID, generation, projectPath, iid, mrURL)
}

// AssignReviewMRTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignReviewMRTaskID(ctx context.Context, watchID string, generation int64, projectPath string, iid int, taskID string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	err := store.AssignReviewMRTaskIDForGeneration(ctx, watchID, generation, projectPath, iid, taskID)
	return s.cleanupTaskAfterOwnershipLoss(ctx, taskID, err)
}

// ReleaseReviewMRTask undoes a reservation (used after task-create failure).
func (s *Service) ReleaseReviewMRTask(ctx context.Context, watchID string, generation int64, projectPath string, iid int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ReleaseReviewMRTaskForGeneration(ctx, watchID, generation, projectPath, iid)
}

// ReserveIssueWatchTask atomically claims an issue for task creation.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID string, generation int64, projectPath string, iid int, issueURL string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	return store.ReserveIssueWatchTaskForGeneration(ctx, watchID, generation, projectPath, iid, issueURL)
}

// AssignIssueWatchTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID string, generation int64, projectPath string, iid int, taskID string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	err := store.AssignIssueWatchTaskIDForGeneration(ctx, watchID, generation, projectPath, iid, taskID)
	return s.cleanupTaskAfterOwnershipLoss(ctx, taskID, err)
}

// ReleaseIssueWatchTask undoes a reservation.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID string, generation int64, projectPath string, iid int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ReleaseIssueWatchTaskForGeneration(ctx, watchID, generation, projectPath, iid)
}

func (s *Service) cleanupTaskAfterOwnershipLoss(ctx context.Context, taskID string, attachErr error) error {
	if !errors.Is(attachErr, ErrWatchOwnershipLost) {
		return attachErr
	}
	s.mu.RLock()
	deleter := s.taskDeleter
	s.mu.RUnlock()
	if deleter == nil {
		return fmt.Errorf("%w: task deleter not configured", attachErr)
	}
	if err := deleter.DeleteTask(ctx, taskID); err != nil {
		return fmt.Errorf("%w: delete unowned task: %v", attachErr, err)
	}
	return attachErr
}
