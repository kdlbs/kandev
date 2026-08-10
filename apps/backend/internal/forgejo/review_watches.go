package forgejo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ReviewWatch struct {
	ID                  string     `json:"id" db:"id"`
	WorkspaceID         string     `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string     `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string     `json:"workflow_step_id" db:"workflow_step_id"`
	RepositoryID        string     `json:"repository_id" db:"repository_id"`
	BaseBranch          string     `json:"base_branch" db:"base_branch"`
	Prompt              string     `json:"prompt" db:"prompt"`
	AgentProfileID      string     `json:"agent_profile_id" db:"agent_profile_id"`
	Owner               string     `json:"owner" db:"owner"`
	Repo                string     `json:"repo" db:"repo"`
	LastError           string     `json:"last_error,omitempty" db:"last_error"`
	Enabled             bool       `json:"enabled" db:"enabled"`
	PollIntervalSeconds int        `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	LastPolledAt        *time.Time `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

func (s *Store) UpsertReviewWatch(ctx context.Context, watch *ReviewWatch) error {
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" || strings.TrimSpace(watch.Owner) == "" || strings.TrimSpace(watch.Repo) == "" {
		return errors.New("forgejo review watch workspace, owner, and repository are required")
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_review_watches (id,workspace_id,workflow_id,workflow_step_id,repository_id,base_branch,prompt,agent_profile_id,owner,repo,enabled,poll_interval_seconds,last_polled_at,last_error,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(workspace_id,owner,repo) DO UPDATE SET workflow_id=excluded.workflow_id,workflow_step_id=excluded.workflow_step_id,repository_id=excluded.repository_id,base_branch=excluded.base_branch,prompt=excluded.prompt,agent_profile_id=excluded.agent_profile_id,enabled=excluded.enabled,poll_interval_seconds=excluded.poll_interval_seconds,updated_at=excluded.updated_at`, watch.ID, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID, watch.RepositoryID, watch.BaseBranch, watch.Prompt, watch.AgentProfileID, watch.Owner, watch.Repo, watch.Enabled, watch.PollIntervalSeconds, watch.LastPolledAt, watch.LastError, watch.CreatedAt, watch.UpdatedAt)
	return err
}
func (s *Store) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	var watches []ReviewWatch
	if err := s.ro.SelectContext(ctx, &watches, `SELECT * FROM forgejo_review_watches ORDER BY created_at`); err != nil {
		return nil, err
	}
	result := make([]*ReviewWatch, len(watches))
	for i := range watches {
		result[i] = &watches[i]
	}
	return result, nil
}
func (s *Store) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	var watches []ReviewWatch
	if err := s.ro.SelectContext(ctx, &watches, `SELECT * FROM forgejo_review_watches WHERE workspace_id=? ORDER BY created_at`, workspaceID); err != nil {
		return nil, err
	}
	result := make([]*ReviewWatch, len(watches))
	for i := range watches {
		result[i] = &watches[i]
	}
	return result, nil
}
func (s *Store) GetReviewWatch(ctx context.Context, workspaceID, id string) (*ReviewWatch, error) {
	var watch ReviewWatch
	if err := s.ro.GetContext(ctx, &watch, `SELECT * FROM forgejo_review_watches WHERE workspace_id=? AND id=?`, workspaceID, id); err != nil {
		return nil, err
	}
	return &watch, nil
}
func (s *Store) DeleteReviewWatch(ctx context.Context, workspaceID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_review_watches WHERE workspace_id=? AND id=?`, workspaceID, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatchNotFound
	}
	return nil
}
func (s *Store) MarkReviewWatchPolled(ctx context.Context, id string, now time.Time, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE forgejo_review_watches SET last_polled_at=?,last_error=?,updated_at=? WHERE id=?`, now, message, now, id)
	return err
}
func (s *Store) ClaimReviewWatchTask(ctx context.Context, w *ReviewWatch, number int) (bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_review_watch_tasks (watch_id,owner,repo,pr_number,created_at) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING`, w.ID, w.Owner, w.Repo, number, time.Now().UTC())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (s *Store) CompleteReviewWatchTask(ctx context.Context, w *ReviewWatch, number int, taskID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE forgejo_review_watch_tasks SET task_id=? WHERE watch_id=? AND owner=? AND repo=? AND pr_number=?`, taskID, w.ID, w.Owner, w.Repo, number)
	return err
}
func (s *Store) ReleaseReviewWatchTask(ctx context.Context, w *ReviewWatch, number int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_review_watch_tasks WHERE watch_id=? AND owner=? AND repo=? AND pr_number=? AND task_id=''`, w.ID, w.Owner, w.Repo, number)
	return err
}
func (s *Service) SaveReviewWatch(ctx context.Context, workspaceID string, watch *ReviewWatch) error {
	if watch == nil {
		return errors.New("forgejo review watch required")
	}
	watch.WorkspaceID = workspaceID
	return s.store.UpsertReviewWatch(ctx, watch)
}
func (s *Service) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.store.ListAllReviewWatches(ctx)
}
func (s *Service) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.store.ListReviewWatches(ctx, workspaceID)
}
func (s *Service) DeleteReviewWatch(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	return s.store.DeleteReviewWatch(ctx, workspaceID, id)
}
func (s *Service) PollReviewWatchByID(ctx context.Context, workspaceID, id string) ([]PullRequest, error) {
	watch, err := s.store.GetReviewWatch(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	return s.PollReviewWatch(ctx, watch)
}
func (s *Service) PollReviewWatch(ctx context.Context, watch *ReviewWatch) ([]PullRequest, error) {
	client, err := s.ClientForWorkspace(ctx, watch.WorkspaceID)
	if err != nil {
		return nil, err
	}
	pulls, _, err := client.ListPullRequests(ctx, watch.Owner, watch.Repo, 1, 100)
	now := time.Now().UTC()
	if err != nil {
		_ = s.store.MarkReviewWatchPolled(ctx, watch.ID, now, "Forgejo review watch poll failed")
		return nil, err
	}
	for _, pull := range pulls {
		if s.reviewTaskCreator == nil || !watch.Enabled || watch.WorkflowID == "" {
			continue
		}
		claimed, err := s.store.ClaimReviewWatchTask(ctx, watch, pull.Number)
		if err != nil {
			return nil, err
		}
		if !claimed {
			continue
		}
		taskID, err := s.reviewTaskCreator(ctx, watch, pull)
		if err != nil {
			_ = s.store.ReleaseReviewWatchTask(ctx, watch, pull.Number)
			_ = s.store.MarkReviewWatchPolled(ctx, watch.ID, now, "Forgejo review watch task creation failed")
			return nil, err
		}
		if err := s.store.CompleteReviewWatchTask(ctx, watch, pull.Number, taskID); err != nil {
			return nil, err
		}
	}
	return pulls, s.store.MarkReviewWatchPolled(ctx, watch.ID, now, "")
}
