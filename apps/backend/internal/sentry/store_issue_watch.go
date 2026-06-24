package sentry

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
	ID                  string        `db:"id"`
	WorkspaceID         string        `db:"workspace_id"`
	InstanceID          string        `db:"sentry_instance_id"`
	WorkflowID          string        `db:"workflow_id"`
	WorkflowStepID      string        `db:"workflow_step_id"`
	FilterJSON          string        `db:"filter_json"`
	AgentProfileID      string        `db:"agent_profile_id"`
	ExecutorProfileID   string        `db:"executor_profile_id"`
	Prompt              string        `db:"prompt"`
	Enabled             bool          `db:"enabled"`
	PollIntervalSeconds int           `db:"poll_interval_seconds"`
	MaxInflightTasks    sql.NullInt64 `db:"max_inflight_tasks"`
	LastPolledAt        *time.Time    `db:"last_polled_at"`
	LastError           string        `db:"last_error"`
	LastErrorAt         *time.Time    `db:"last_error_at"`
	CreatedAt           time.Time     `db:"created_at"`
	UpdatedAt           time.Time     `db:"updated_at"`
}

func (r *issueWatchRow) toIssueWatch() (*IssueWatch, error) {
	var filter SearchFilter
	if r.FilterJSON != "" {
		if err := json.Unmarshal([]byte(r.FilterJSON), &filter); err != nil {
			return nil, fmt.Errorf("decode filter: %w", err)
		}
	}
	var maxInflight *int
	if r.MaxInflightTasks.Valid {
		v := int(r.MaxInflightTasks.Int64)
		maxInflight = &v
	}
	return &IssueWatch{
		ID:                  r.ID,
		WorkspaceID:         r.WorkspaceID,
		InstanceID:          r.InstanceID,
		WorkflowID:          r.WorkflowID,
		WorkflowStepID:      r.WorkflowStepID,
		Filter:              filter,
		AgentProfileID:      r.AgentProfileID,
		ExecutorProfileID:   r.ExecutorProfileID,
		Prompt:              r.Prompt,
		Enabled:             r.Enabled,
		PollIntervalSeconds: r.PollIntervalSeconds,
		MaxInflightTasks:    maxInflight,
		LastPolledAt:        r.LastPolledAt,
		LastError:           r.LastError,
		LastErrorAt:         r.LastErrorAt,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}, nil
}

// nullableInt converts a *int into a value suitable for a nullable SQL column.
// A nil pointer becomes a SQL NULL; a non-nil pointer becomes the underlying
// int. Used for max_inflight_tasks where NULL means "uncapped".
func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func encodeFilter(f SearchFilter) (string, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return "", fmt.Errorf("encode filter: %w", err)
	}
	return string(b), nil
}

// issueWatchInsertColumns lists the writable column names in row-insert order.
// SELECTs use issueWatchSelectColumns which wraps the nullable last_error in
// COALESCE so older databases (pre-self-heal migration) read back as empty
// strings rather than NULL.
const issueWatchInsertColumns = `id, workspace_id, sentry_instance_id, workflow_id, workflow_step_id, filter_json,
	agent_profile_id, executor_profile_id, prompt, enabled,
	poll_interval_seconds, max_inflight_tasks, last_polled_at,
	last_error, last_error_at,
	created_at, updated_at`

const issueWatchSelectColumns = `id, workspace_id, sentry_instance_id, workflow_id, workflow_step_id, filter_json,
	agent_profile_id, executor_profile_id, prompt, enabled,
	poll_interval_seconds, max_inflight_tasks, last_polled_at,
	COALESCE(last_error, '') AS last_error, last_error_at,
	created_at, updated_at`

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
		INSERT INTO sentry_issue_watches (`+issueWatchInsertColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.WorkspaceID, w.InstanceID, w.WorkflowID, w.WorkflowStepID, filterJSON,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt, w.Enabled,
		w.PollIntervalSeconds, nullableInt(w.MaxInflightTasks), w.LastPolledAt,
		w.LastError, w.LastErrorAt,
		w.CreatedAt, w.UpdatedAt)
	return err
}

// GetIssueWatch returns a single watch by ID, or nil when no row matches.
func (s *Store) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	var row issueWatchRow
	err := s.ro.GetContext(ctx, &row,
		`SELECT `+issueWatchSelectColumns+` FROM sentry_issue_watches WHERE id = ?`, id)
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
		`SELECT `+issueWatchSelectColumns+` FROM sentry_issue_watches
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
		`SELECT `+issueWatchSelectColumns+` FROM sentry_issue_watches ORDER BY workspace_id, created_at`)
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
		`SELECT `+issueWatchSelectColumns+` FROM sentry_issue_watches
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
// updated_at is bumped automatically. last_polled_at is deliberately NOT in the
// UPDATE: that column is owned exclusively by UpdateIssueWatchLastPolled, so a
// concurrent poller stamp can't be clobbered by a load-modify-write edit that
// carried a stale (often nil) pre-fetch value.
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
		UPDATE sentry_issue_watches SET sentry_instance_id = ?, workflow_id = ?, workflow_step_id = ?, filter_json = ?,
			agent_profile_id = ?, executor_profile_id = ?, prompt = ?,
			enabled = ?, poll_interval_seconds = ?, max_inflight_tasks = ?, updated_at = ?
		WHERE id = ?`,
		w.InstanceID, w.WorkflowID, w.WorkflowStepID, filterJSON,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt,
		w.Enabled, w.PollIntervalSeconds, nullableInt(w.MaxInflightTasks), w.UpdatedAt, w.ID)
	return err
}

// UpdateIssueWatchLastPolled stamps the last-polled timestamp without touching
// the rest of the row.
func (s *Store) UpdateIssueWatchLastPolled(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sentry_issue_watches SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
		t, time.Now().UTC(), id)
	return err
}

// DisableIssueWatchWithError flips a watch to disabled and stamps a
// human-readable cause. Called by the dispatch coordinator's self-heal path
// when the watch's bound agent profile has been soft-deleted, so the settings
// UI can explain why the watch stopped firing. Mirrors the Linear/Jira stores.
func (s *Store) DisableIssueWatchWithError(ctx context.Context, id, cause string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE sentry_issue_watches
		   SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ?
		 WHERE id = ?`,
		cause, now, now, id)
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM sentry_issue_watch_tasks WHERE issue_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sentry_issue_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReserveIssueWatchTask atomically claims a slot for a (watch, issue) pair via
// INSERT OR IGNORE. Returns true when this caller won the race and should
// proceed to create the task.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, watchID, shortID, issueURL string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO sentry_issue_watch_tasks (id, issue_watch_id, issue_short_id, issue_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), watchID, shortID, issueURL, "", time.Now().UTC())
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
func (s *Store) AssignIssueWatchTaskID(ctx context.Context, watchID, shortID, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sentry_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND issue_short_id = ?`,
		taskID, watchID, shortID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("assign task ID: reservation row not found for watch=%s issue=%s", watchID, shortID)
	}
	return nil
}

// ReleaseIssueWatchTask drops a reservation so the next poll can retry. Used
// when task creation fails after a successful reserve.
func (s *Store) ReleaseIssueWatchTask(ctx context.Context, watchID, shortID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sentry_issue_watch_tasks WHERE issue_watch_id = ? AND issue_short_id = ?`,
		watchID, shortID)
	return err
}

// ListSeenIssueShortIDs returns the set of issue short IDs already reserved
// against a watch.
func (s *Store) ListSeenIssueShortIDs(ctx context.Context, watchID string) (map[string]struct{}, error) {
	var keys []string
	err := s.ro.SelectContext(ctx, &keys,
		`SELECT issue_short_id FROM sentry_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out, nil
}

// ListIssueWatchTaskIDs returns every task_id recorded against a watch,
// including the empty-string sentinel reservations that never completed.
// Used by the reset flow to enumerate the tasks to cascade-delete.
func (s *Store) ListIssueWatchTaskIDs(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM sentry_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	return ids, err
}

// ResetIssueWatchState wipes the watch's dedup rows and nulls its
// last_polled_at in one transaction. Used by the reset flow after the
// cascade-delete loop so the next poll re-imports every currently-matching
// issue as if the watch were freshly created.
func (s *Store) ResetIssueWatchState(ctx context.Context, watchID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sentry_issue_watch_tasks WHERE issue_watch_id = ?`, watchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE sentry_issue_watches SET last_polled_at = NULL, updated_at = ? WHERE id = ?`,
		time.Now().UTC(), watchID); err != nil {
		return err
	}
	return tx.Commit()
}
