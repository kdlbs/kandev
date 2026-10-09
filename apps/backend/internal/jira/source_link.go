package jira

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrSourceLinkNotFound  = errors.New("jira: task has no linked issue")
	ErrSourceLinkAmbiguous = errors.New("jira: task has multiple linked issues")
)

// TaskIssueLink is the host-owned association between a task and its Jira issue.
type TaskIssueLink struct {
	WorkspaceID string
	IssueKey    string
	IssueURL    string
}

// GetTaskIssueLink resolves only issues attached through a persisted Jira watch.
func (s *Service) GetTaskIssueLink(ctx context.Context, taskID string) (*TaskIssueLink, error) {
	return s.store.GetTaskIssueLink(ctx, taskID)
}

func (s *Store) GetTaskIssueLink(ctx context.Context, taskID string) (*TaskIssueLink, error) {
	if taskID == "" {
		return nil, ErrSourceLinkNotFound
	}
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT DISTINCT w.workspace_id, t.issue_key, t.issue_url
		FROM jira_issue_watch_tasks t
		JOIN jira_issue_watches w ON w.id = t.issue_watch_id
		WHERE t.task_id = ?
		ORDER BY w.workspace_id, t.issue_key, t.issue_url
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var link *TaskIssueLink
	for rows.Next() {
		candidate := &TaskIssueLink{}
		if err := rows.Scan(&candidate.WorkspaceID, &candidate.IssueKey, &candidate.IssueURL); err != nil {
			return nil, err
		}
		if link != nil {
			return nil, ErrSourceLinkAmbiguous
		}
		link = candidate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if link == nil {
		return nil, ErrSourceLinkNotFound
	}
	return link, nil
}

// ValidateTaskIssueLink checks that the supplied identity still matches the
// persisted association in this workspace.
func (s *Service) ValidateTaskIssueLink(ctx context.Context, taskID, workspaceID, issueKey string) error {
	link, err := s.GetTaskIssueLink(ctx, taskID)
	if err != nil {
		return err
	}
	if link.WorkspaceID != workspaceID || link.IssueKey != issueKey {
		return fmt.Errorf("%w: task source association changed", ErrSourceLinkNotFound)
	}
	return nil
}
