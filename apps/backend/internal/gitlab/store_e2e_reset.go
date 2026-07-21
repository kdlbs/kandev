package gitlab

import "context"

// E2EResetResult summarizes the workspace-scoped poller definitions removed
// before the shared E2E task reset starts deleting tasks.
type E2EResetResult struct {
	ReviewWatches int `json:"review_watches"`
	IssueWatches  int `json:"issue_watches"`
}

// ResetWorkspaceE2E removes all workspace-owned GitLab state except the
// connection. The service deletes the connection separately so its secret can
// be removed through the credential store instead of bypassing it with SQL.
func (s *Store) ResetWorkspaceE2E(ctx context.Context, workspaceID string) (E2EResetResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return E2EResetResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	queries := []string{
		`DELETE FROM gitlab_review_mr_tasks WHERE review_watch_id IN
			(SELECT id FROM gitlab_review_watches WHERE workspace_id = ?)`,
		`DELETE FROM gitlab_issue_watch_tasks WHERE issue_watch_id IN
			(SELECT id FROM gitlab_issue_watches WHERE workspace_id = ?)`,
	}
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query, workspaceID); err != nil {
			return E2EResetResult{}, err
		}
	}

	reviewResult, err := tx.ExecContext(ctx,
		`DELETE FROM gitlab_review_watches WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return E2EResetResult{}, err
	}
	issueResult, err := tx.ExecContext(ctx,
		`DELETE FROM gitlab_issue_watches WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return E2EResetResult{}, err
	}

	workspaceDeletes := []string{
		`DELETE FROM gitlab_action_presets WHERE workspace_id = ?`,
		`DELETE FROM gitlab_mr_watches WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`,
		`DELETE FROM gitlab_task_mrs WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`,
	}
	for _, query := range workspaceDeletes {
		if _, err := tx.ExecContext(ctx, query, workspaceID); err != nil {
			return E2EResetResult{}, err
		}
	}

	reviewCount, err := reviewResult.RowsAffected()
	if err != nil {
		return E2EResetResult{}, err
	}
	issueCount, err := issueResult.RowsAffected()
	if err != nil {
		return E2EResetResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EResetResult{}, err
	}
	return E2EResetResult{ReviewWatches: int(reviewCount), IssueWatches: int(issueCount)}, nil
}

// ResetWorkspaceE2E clears provider mock data, poller state, durable links,
// presets, and the workspace connection. It is called only by the backend's
// test-gated E2E reset route.
func (s *Service) ResetWorkspaceE2E(ctx context.Context, workspaceID string) (E2EResetResult, error) {
	s.mu.RLock()
	store := s.store
	cachedClient := s.workspaceClients[workspaceID]
	s.mu.RUnlock()
	if store == nil {
		return E2EResetResult{}, ErrNotConfigured
	}
	result, err := store.ResetWorkspaceE2E(ctx, workspaceID)
	if err != nil {
		return E2EResetResult{}, err
	}
	if mock, ok := cachedClient.(*MockClient); ok {
		mock.Reset()
	}
	if err := s.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return result, err
	}
	return result, nil
}
