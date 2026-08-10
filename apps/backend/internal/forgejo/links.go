package forgejo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrTaskLinkNotFound = errors.New("forgejo: task link not found")

// TaskIssue is a durable Forgejo issue association for a Kandev task.
type TaskIssue struct {
	ID           string     `json:"id" db:"id"`
	TaskID       string     `json:"task_id" db:"task_id"`
	RepositoryID string     `json:"repository_id,omitempty" db:"repository_id"`
	Origin       string     `json:"origin" db:"origin"`
	Owner        string     `json:"owner" db:"owner"`
	Repo         string     `json:"repo" db:"repo"`
	IssueNumber  int        `json:"issue_number" db:"issue_number"`
	IssueURL     string     `json:"issue_url" db:"issue_url"`
	Title        string     `json:"title" db:"title"`
	State        string     `json:"state" db:"state"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskPR is a durable Forgejo pull-request association for a Kandev task.
type TaskPR struct {
	ID           string     `json:"id" db:"id"`
	TaskID       string     `json:"task_id" db:"task_id"`
	RepositoryID string     `json:"repository_id,omitempty" db:"repository_id"`
	Origin       string     `json:"origin" db:"origin"`
	Owner        string     `json:"owner" db:"owner"`
	Repo         string     `json:"repo" db:"repo"`
	PRNumber     int        `json:"pr_number" db:"pr_number"`
	PRURL        string     `json:"pr_url" db:"pr_url"`
	PRTitle      string     `json:"pr_title" db:"pr_title"`
	HeadBranch   string     `json:"head_branch" db:"head_branch"`
	BaseBranch   string     `json:"base_branch" db:"base_branch"`
	State        string     `json:"state" db:"state"`
	Draft        bool       `json:"draft" db:"draft"`
	Mergeable    bool       `json:"mergeable" db:"mergeable"`
	CIState      string     `json:"ci_state" db:"ci_state"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

func (s *Store) UpsertTaskIssue(ctx context.Context, issue *TaskIssue) error {
	if issue == nil || strings.TrimSpace(issue.TaskID) == "" || strings.TrimSpace(issue.Owner) == "" || strings.TrimSpace(issue.Repo) == "" || issue.IssueNumber <= 0 {
		return errors.New("forgejo task issue identity required")
	}
	if issue.ID == "" {
		issue.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if issue.CreatedAt.IsZero() {
		issue.CreatedAt = now
	}
	issue.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_task_issues (id, task_id, repository_id, origin, owner, repo, issue_number, issue_url, title, state, last_synced_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, owner, repo, issue_number) DO UPDATE SET origin=excluded.origin, issue_url=excluded.issue_url, title=excluded.title, state=excluded.state, last_synced_at=excluded.last_synced_at, updated_at=excluded.updated_at`,
		issue.ID, issue.TaskID, issue.RepositoryID, issue.Origin, issue.Owner, issue.Repo, issue.IssueNumber, issue.IssueURL, issue.Title, issue.State, issue.LastSyncedAt, issue.CreatedAt, issue.UpdatedAt)
	return err
}

func (s *Store) UpsertTaskPR(ctx context.Context, pr *TaskPR) error {
	if pr == nil || strings.TrimSpace(pr.TaskID) == "" || strings.TrimSpace(pr.Owner) == "" || strings.TrimSpace(pr.Repo) == "" || pr.PRNumber <= 0 {
		return errors.New("forgejo task pull request identity required")
	}
	if pr.ID == "" {
		pr.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = now
	}
	pr.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_task_prs (id, task_id, repository_id, origin, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, state, draft, mergeable, ci_state, last_synced_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, owner, repo, pr_number) DO UPDATE SET origin=excluded.origin, pr_url=excluded.pr_url, pr_title=excluded.pr_title, head_branch=excluded.head_branch, base_branch=excluded.base_branch, state=excluded.state, draft=excluded.draft, mergeable=excluded.mergeable, ci_state=excluded.ci_state, last_synced_at=excluded.last_synced_at, updated_at=excluded.updated_at`,
		pr.ID, pr.TaskID, pr.RepositoryID, pr.Origin, pr.Owner, pr.Repo, pr.PRNumber, pr.PRURL, pr.PRTitle, pr.HeadBranch, pr.BaseBranch, pr.State, pr.Draft, pr.Mergeable, pr.CIState, pr.LastSyncedAt, pr.CreatedAt, pr.UpdatedAt)
	return err
}

func (s *Store) ListTaskIssues(ctx context.Context, workspaceID, taskID string) ([]*TaskIssue, error) {
	if err := s.assertTaskWorkspace(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	var issues []TaskIssue
	if err := s.ro.SelectContext(ctx, &issues, `SELECT * FROM forgejo_task_issues WHERE task_id = ? ORDER BY created_at`, taskID); err != nil {
		return nil, err
	}
	result := make([]*TaskIssue, len(issues))
	for i := range issues {
		result[i] = &issues[i]
	}
	return result, nil
}

func (s *Store) ListTaskPRs(ctx context.Context, workspaceID, taskID string) ([]*TaskPR, error) {
	if err := s.assertTaskWorkspace(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs, `SELECT * FROM forgejo_task_prs WHERE task_id = ? ORDER BY created_at`, taskID); err != nil {
		return nil, err
	}
	result := make([]*TaskPR, len(prs))
	for i := range prs {
		result[i] = &prs[i]
	}
	return result, nil
}

func (s *Store) DeleteTaskIssue(ctx context.Context, workspaceID, linkID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_task_issues WHERE id = ? AND task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`, linkID, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTaskLinkNotFound
	}
	return nil
}

func (s *Store) DeleteTaskPR(ctx context.Context, workspaceID, linkID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_task_prs WHERE id = ? AND task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`, linkID, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTaskLinkNotFound
	}
	return nil
}

func (s *Store) assertTaskWorkspace(ctx context.Context, workspaceID, taskID string) error {
	var found int
	err := s.ro.GetContext(ctx, &found, `SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ?`, taskID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskLinkNotFound
	}
	if err != nil {
		return fmt.Errorf("validate Forgejo task link workspace: %w", err)
	}
	return nil
}
