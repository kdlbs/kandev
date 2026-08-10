package forgejo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrWatchNotFound = errors.New("forgejo: issue watch not found")

type IssueWatch struct {
	ID                  string     `json:"id" db:"id"`
	WorkspaceID         string     `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string     `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string     `json:"workflow_step_id" db:"workflow_step_id"`
	RepositoryID        string     `json:"repository_id" db:"repository_id"`
	BaseBranch          string     `json:"base_branch" db:"base_branch"`
	Prompt              string     `json:"prompt" db:"prompt"`
	Owner               string     `json:"owner" db:"owner"`
	Repo                string     `json:"repo" db:"repo"`
	Labels              string     `json:"labels" db:"labels"`
	Enabled             bool       `json:"enabled" db:"enabled"`
	PollIntervalSeconds int        `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	LastPolledAt        *time.Time `json:"last_polled_at,omitempty" db:"last_polled_at"`
	LastError           string     `json:"last_error,omitempty" db:"last_error"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

func (s *Store) UpsertIssueWatch(ctx context.Context, watch *IssueWatch) error {
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" || strings.TrimSpace(watch.Owner) == "" || strings.TrimSpace(watch.Repo) == "" {
		return errors.New("forgejo issue watch workspace, owner, and repository are required")
	}
	if watch.ID == "" {
		watch.ID = uuid.NewString()
	}
	if watch.PollIntervalSeconds < 30 {
		watch.PollIntervalSeconds = 300
	}
	now := time.Now().UTC()
	if watch.CreatedAt.IsZero() {
		watch.CreatedAt = now
	}
	watch.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_issue_watches (id, workspace_id, workflow_id, workflow_step_id, repository_id, base_branch, prompt, owner, repo, labels, enabled, poll_interval_seconds, last_polled_at, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id, owner, repo, labels) DO UPDATE SET workflow_id=excluded.workflow_id, workflow_step_id=excluded.workflow_step_id, repository_id=excluded.repository_id, base_branch=excluded.base_branch, prompt=excluded.prompt, enabled=excluded.enabled, poll_interval_seconds=excluded.poll_interval_seconds, updated_at=excluded.updated_at`, watch.ID, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID, watch.RepositoryID, watch.BaseBranch, watch.Prompt, watch.Owner, watch.Repo, watch.Labels, watch.Enabled, watch.PollIntervalSeconds, watch.LastPolledAt, watch.LastError, watch.CreatedAt, watch.UpdatedAt)
	return err
}

func (s *Store) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	var watches []IssueWatch
	if err := s.ro.SelectContext(ctx, &watches, `SELECT * FROM forgejo_issue_watches WHERE workspace_id = ? ORDER BY created_at`, workspaceID); err != nil {
		return nil, err
	}
	result := make([]*IssueWatch, len(watches))
	for i := range watches {
		result[i] = &watches[i]
	}
	return result, nil
}

func (s *Store) DeleteIssueWatch(ctx context.Context, workspaceID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_issue_watches WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchNotFound
	}
	return nil
}

func (s *Store) MarkIssueWatchPolled(ctx context.Context, id string, pollTime time.Time, lastError string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE forgejo_issue_watches SET last_polled_at = ?, last_error = ?, updated_at = ? WHERE id = ?`, pollTime, lastError, pollTime, id)
	return err
}

func (s *Store) GetIssueWatch(ctx context.Context, workspaceID, id string) (*IssueWatch, error) {
	var watch IssueWatch
	err := s.ro.GetContext(ctx, &watch, `SELECT * FROM forgejo_issue_watches WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWatchNotFound
	}
	if err != nil {
		return nil, err
	}
	return &watch, nil
}

func filterWatchedIssues(issues []Issue, labels string) []Issue {
	labels = strings.TrimSpace(labels)
	if labels == "" {
		return issues
	}
	wanted := map[string]bool{}
	for _, label := range strings.Split(labels, ",") {
		wanted[strings.TrimSpace(label)] = true
	}
	result := make([]Issue, 0)
	for _, issue := range issues {
		for _, label := range issue.Labels {
			if wanted[label] {
				result = append(result, issue)
				break
			}
		}
	}
	return result
}

func isMissingWatch(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrWatchNotFound)
}
