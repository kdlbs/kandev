package gitlab

import "context"

// ReserveReviewMRTask atomically claims an MR for task creation. Returns
// (true, nil) when this caller wins the race, (false, nil) when another
// caller has already reserved (or task created).
func (s *Service) ReserveReviewMRTask(ctx context.Context, watchID, projectPath string, iid int, mrURL string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	return store.ReserveReviewMRTask(ctx, watchID, projectPath, iid, mrURL)
}

// AssignReviewMRTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignReviewMRTaskID(ctx context.Context, watchID, projectPath string, iid int, taskID string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.AssignReviewMRTaskID(ctx, watchID, projectPath, iid, taskID)
}

// ReleaseReviewMRTask undoes a reservation (used after task-create failure).
func (s *Service) ReleaseReviewMRTask(ctx context.Context, watchID, projectPath string, iid int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ReleaseReviewMRTask(ctx, watchID, projectPath, iid)
}

// ReserveIssueWatchTask atomically claims an issue for task creation.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, projectPath string, iid int, issueURL string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	return store.ReserveIssueWatchTask(ctx, watchID, projectPath, iid, issueURL)
}

// AssignIssueWatchTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID, projectPath string, iid int, taskID string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.AssignIssueWatchTaskID(ctx, watchID, projectPath, iid, taskID)
}

// ReleaseIssueWatchTask undoes a reservation.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID, projectPath string, iid int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ReleaseIssueWatchTask(ctx, watchID, projectPath, iid)
}
