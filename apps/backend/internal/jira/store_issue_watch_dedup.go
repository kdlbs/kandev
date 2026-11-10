package jira

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// ReserveIssueWatchTask atomically claims a slot for a (watch, issue) pair.
// A mapping whose task was permanently deleted is removed in the same
// transaction; an unfinished reservation or a mapping to an existing task
// remains protected and makes this caller lose the claim.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, watchID, issueKey, issueURL string) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin Jira issue claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockJiraIssueWatchClaim(ctx, tx, s.db.DriverName(), watchID, issueKey); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM jira_issue_watch_tasks
		WHERE issue_watch_id = ? AND issue_key = ? AND task_id <> ''
		  AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = jira_issue_watch_tasks.task_id)`),
		watchID, issueKey); err != nil {
		return false, fmt.Errorf("reclaim deleted Jira issue task: %w", err)
	}

	res, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO jira_issue_watch_tasks (id, issue_watch_id, issue_key, issue_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(issue_watch_id, issue_key) DO NOTHING`),
		uuid.New().String(), watchID, issueKey, issueURL, "", time.Now().UTC())
	if err != nil {
		return false, fmt.Errorf("reserve Jira issue task: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count Jira issue reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit Jira issue claim: %w", err)
	}
	return rows == 1, nil
}

// ListSeenIssueKeys returns the set of issue keys whose mapping still
// suppresses dispatch for this watch. Empty reservations protect unfinished
// task creation; assigned mappings protect every task that still exists,
// including archived tasks and tasks on completed steps.
func (s *Store) ListSeenIssueKeys(ctx context.Context, watchID string) (map[string]struct{}, error) {
	var keys []string
	err := s.ro.SelectContext(ctx, &keys, s.ro.Rebind(`
		SELECT issue_key FROM jira_issue_watch_tasks d
		WHERE d.issue_watch_id = ?
		  AND (d.task_id = '' OR EXISTS (SELECT 1 FROM tasks t WHERE t.id = d.task_id))`),
		watchID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out, nil
}

// lockJiraIssueWatchClaim serializes claims for one watch and issue. SQLite
// acquires its single writer at BEGIN; PostgreSQL needs an explicit
// transaction-scoped lock because it admits concurrent writers.
func lockJiraIssueWatchClaim(ctx context.Context, tx *sqlx.Tx, driver, watchID, issueKey string) error {
	if !dialect.IsPostgres(driver) {
		return nil
	}
	key := fmt.Sprintf("jira-issue-watch-claim:%q:%q", watchID, issueKey)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("acquire Jira issue watch claim lock: %w", err)
	}
	return nil
}
