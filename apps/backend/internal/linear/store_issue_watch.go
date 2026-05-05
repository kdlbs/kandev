package linear

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// issueWatchRow mirrors IssueWatch but stores the filter as a raw JSON string
// the way SQLite holds it. The store marshals/unmarshals at this boundary so
// service callers see a typed SearchFilter and never have to think about JSON.
type issueWatchRow struct {
	ID                  string     `db:"id"`
	WorkspaceID         string     `db:"workspace_id"`
	WorkflowID          string     `db:"workflow_id"`
	WorkflowStepID      string     `db:"workflow_step_id"`
	FilterJSON          string     `db:"filter_json"`
	AgentProfileID      string     `db:"agent_profile_id"`
	ExecutorProfileID   string     `db:"executor_profile_id"`
	Prompt              string     `db:"prompt"`
	Enabled             bool       `db:"enabled"`
	PollIntervalSeconds int        `db:"poll_interval_seconds"`
	LastPolledAt        *time.Time `db:"last_polled_at"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
}

func (r *issueWatchRow) toIssueWatch() (*IssueWatch, error) {
	var filter SearchFilter
	if r.FilterJSON != "" {
		if err := json.Unmarshal([]byte(r.FilterJSON), &filter); err != nil {
			return nil, fmt.Errorf("decode filter: %w", err)
		}
	}
	return &IssueWatch{
		ID:                  r.ID,
		WorkspaceID:         r.WorkspaceID,
		WorkflowID:          r.WorkflowID,
		WorkflowStepID:      r.WorkflowStepID,
		Filter:              filter,
		AgentProfileID:      r.AgentProfileID,
		ExecutorProfileID:   r.ExecutorProfileID,
		Prompt:              r.Prompt,
		Enabled:             r.Enabled,
		PollIntervalSeconds: r.PollIntervalSeconds,
		LastPolledAt:        r.LastPolledAt,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}, nil
}

func encodeFilter(f SearchFilter) (string, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return "", fmt.Errorf("encode filter: %w", err)
	}
	return string(b), nil
}

const issueWatchColumns = `id, workspace_id, workflow_id, workflow_step_id, filter_json,
	agent_profile_id, executor_profile_id, prompt, enabled,
	poll_interval_seconds, last_polled_at, created_at, updated_at`

// CreateIssueWatch persists a new issue watch row. ID and timestamps are
// assigned here so callers can pass a partially-populated struct.
func (s *Store) CreateIssueWatch(ctx context.Context, w *IssueWatch) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	if w.PollIntervalSeconds <= 0 {
		w.PollIntervalSeconds = DefaultIssueWatchPollInterval
	}
	filterJSON, err := encodeFilter(w.Filter)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO linear_issue_watches (`+issueWatchColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.WorkspaceID, w.WorkflowID, w.WorkflowStepID, filterJSON,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt, w.Enabled,
		w.PollIntervalSeconds, w.LastPolledAt, w.CreatedAt, w.UpdatedAt)
	return err
}

// GetIssueWatch returns a single watch by ID, or nil when no row matches.
func (s *Store) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	var row issueWatchRow
	err := s.ro.GetContext(ctx, &row,
		`SELECT `+issueWatchColumns+` FROM linear_issue_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.toIssueWatch()
}

// ListIssueWatches returns all watches configured for a workspace.
func (s *Store) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	var rows []issueWatchRow
	err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+issueWatchColumns+` FROM linear_issue_watches
		 WHERE workspace_id = ? ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	return materializeWatches(rows)
}

// ListAllIssueWatches returns every watch across all workspaces.
func (s *Store) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var rows []issueWatchRow
	err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+issueWatchColumns+` FROM linear_issue_watches ORDER BY workspace_id, created_at`)
	if err != nil {
		return nil, err
	}
	return materializeWatches(rows)
}

// ListEnabledIssueWatches returns every enabled watch across all workspaces,
// used by the poller to decide what to query each tick.
func (s *Store) ListEnabledIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var rows []issueWatchRow
	err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+issueWatchColumns+` FROM linear_issue_watches
		 WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return materializeWatches(rows)
}

func materializeWatches(rows []issueWatchRow) ([]*IssueWatch, error) {
	out := make([]*IssueWatch, 0, len(rows))
	for i := range rows {
		w, err := rows[i].toIssueWatch()
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// UpdateIssueWatch overwrites the mutable fields of an existing watch row.
// updated_at is bumped automatically; last_polled_at is preserved unless the
// caller explicitly sets it.
func (s *Store) UpdateIssueWatch(ctx context.Context, w *IssueWatch) error {
	w.UpdatedAt = time.Now().UTC()
	if w.PollIntervalSeconds <= 0 {
		w.PollIntervalSeconds = DefaultIssueWatchPollInterval
	}
	filterJSON, err := encodeFilter(w.Filter)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE linear_issue_watches SET workflow_id = ?, workflow_step_id = ?, filter_json = ?,
			agent_profile_id = ?, executor_profile_id = ?, prompt = ?,
			enabled = ?, poll_interval_seconds = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		w.WorkflowID, w.WorkflowStepID, filterJSON,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt,
		w.Enabled, w.PollIntervalSeconds, w.LastPolledAt, w.UpdatedAt, w.ID)
	return err
}

// UpdateIssueWatchLastPolled stamps the last-polled timestamp without touching
// the rest of the row.
func (s *Store) UpdateIssueWatchLastPolled(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE linear_issue_watches SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
		t, time.Now().UTC(), id)
	return err
}

// DeleteIssueWatch removes a watch and its dedup rows in a single transaction.
// The explicit child DELETE guards older databases where foreign_keys may not
// have been enabled at attach time.
func (s *Store) DeleteIssueWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM linear_issue_watch_tasks WHERE issue_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM linear_issue_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReserveIssueWatchTask atomically claims a slot for a (watch, issue) pair via
// INSERT OR IGNORE. Returns true when this caller won the race and should
// proceed to create the task.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, watchID, identifier, issueURL string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO linear_issue_watch_tasks (id, issue_watch_id, issue_identifier, issue_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), watchID, identifier, issueURL, "", time.Now().UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// AssignIssueWatchTaskID stamps the created task ID onto a previously-reserved
// dedup row.
func (s *Store) AssignIssueWatchTaskID(ctx context.Context, watchID, identifier, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE linear_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND issue_identifier = ?`,
		taskID, watchID, identifier)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("assign task ID: reservation row not found for watch=%s issue=%s", watchID, identifier)
	}
	return nil
}

// ReleaseIssueWatchTask drops a reservation so the next poll can retry. Used
// when task creation fails after a successful reserve.
func (s *Store) ReleaseIssueWatchTask(ctx context.Context, watchID, identifier string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM linear_issue_watch_tasks WHERE issue_watch_id = ? AND issue_identifier = ?`,
		watchID, identifier)
	return err
}

// ListSeenIssueIdentifiers returns the set of issue identifiers already
// reserved against a watch.
func (s *Store) ListSeenIssueIdentifiers(ctx context.Context, watchID string) (map[string]struct{}, error) {
	var keys []string
	err := s.ro.SelectContext(ctx, &keys,
		`SELECT issue_identifier FROM linear_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out, nil
}
