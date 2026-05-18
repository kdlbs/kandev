package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// GetRun fetches a single run row by id.
func (r *Repository) GetRun(
	ctx context.Context, id string,
) (*models.Run, error) {
	var req models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs WHERE id = ?
	`), id).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// DefaultAgentFailureThreshold is the workspace-level default applied
// when neither a per-workspace nor per-agent override is set.
const DefaultAgentFailureThreshold = 3

// IncrementAgentConsecutiveFailures bumps the per-agent counter and
// returns the post-increment value so callers can decide whether the
// threshold has been crossed.
func (r *Repository) IncrementAgentConsecutiveFailures(
	ctx context.Context, agentID string,
) (int, error) {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET consecutive_failures = consecutive_failures + 1,
		    updated_at = ?
		WHERE id = ?
	`), time.Now().UTC(), agentID); err != nil {
		return 0, err
	}
	var n int
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT consecutive_failures FROM agent_profiles WHERE id = ?
	`), agentID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ResetAgentConsecutiveFailures clears the counter — called after any
// successful agent turn or when the user marks a paused agent as
// fixed via the inbox.
func (r *Repository) ResetAgentConsecutiveFailures(
	ctx context.Context, agentID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET consecutive_failures = 0, updated_at = ?
		WHERE id = ?
	`), time.Now().UTC(), agentID)
	return err
}

// GetEffectiveFailureThreshold resolves the per-agent override, then
// the per-workspace override, then the global default.
//
// agent_profiles.failure_threshold is NOT NULL DEFAULT 3 under the
// merged schema; office stores 0 to mean "no override, use workspace
// default" (round-tripped to NULL on the AgentInstance struct).
func (r *Repository) GetEffectiveFailureThreshold(
	ctx context.Context, agentID string,
) (int, error) {
	var override sql.NullInt64
	var workspaceID string
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT NULLIF(failure_threshold, 0), COALESCE(workspace_id, '')
		FROM agent_profiles WHERE id = ?
	`), agentID).Scan(&override, &workspaceID); err != nil {
		return 0, err
	}
	if override.Valid && override.Int64 > 0 {
		return int(override.Int64), nil
	}
	return r.GetWorkspaceAgentFailureThreshold(ctx, workspaceID)
}

// GetWorkspaceAgentFailureThreshold returns the workspace-level
// threshold, falling back to DefaultAgentFailureThreshold when the
// workspace has no row.
func (r *Repository) GetWorkspaceAgentFailureThreshold(
	ctx context.Context, workspaceID string,
) (int, error) {
	var n int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT agent_failure_threshold
		FROM office_workspace_settings WHERE workspace_id = ?
	`), workspaceID).Scan(&n)
	if err == sql.ErrNoRows {
		return DefaultAgentFailureThreshold, nil
	}
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return DefaultAgentFailureThreshold, nil
	}
	return n, nil
}

// SetWorkspaceAgentFailureThreshold upserts the workspace setting.
func (r *Repository) SetWorkspaceAgentFailureThreshold(
	ctx context.Context, workspaceID string, threshold int,
) error {
	if threshold <= 0 {
		threshold = DefaultAgentFailureThreshold
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_workspace_settings (workspace_id, agent_failure_threshold, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			agent_failure_threshold = excluded.agent_failure_threshold,
			updated_at = excluded.updated_at
	`), workspaceID, threshold, time.Now().UTC())
	return err
}

// MarkRunFailed sets a run row to status=failed with a verbatim
// error message and stamps finished_at. Idempotent: re-marking is a
// no-op when the run is already in the same state.
func (r *Repository) MarkRunFailed(
	ctx context.Context, runID, errorMessage string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'failed',
		    error_message = ?,
		    finished_at = COALESCE(finished_at, ?)
		WHERE id = ?
	`), errorMessage, time.Now().UTC(), runID)
	return err
}

// DismissInboxItem records a per-user dismissal. Idempotent — a second
// dismiss for the same (user, kind, item) is silently accepted.
func (r *Repository) DismissInboxItem(
	ctx context.Context, userID, kind, itemID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_inbox_dismissals (user_id, item_kind, item_id, dismissed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, item_kind, item_id) DO UPDATE SET
			dismissed_at = excluded.dismissed_at
	`), userID, kind, itemID, time.Now().UTC())
	return err
}

// IsInboxItemDismissed reports whether the given (user, kind, item)
// has been dismissed.
func (r *Repository) IsInboxItemDismissed(
	ctx context.Context, userID, kind, itemID string,
) (bool, error) {
	var n int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM office_inbox_dismissals
		WHERE user_id = ? AND item_kind = ? AND item_id = ?
	`), userID, kind, itemID).Scan(&n)
	return n > 0, err
}

// ListDismissedInboxItemIDs returns the set of item ids dismissed by
// the given user for the given kind. Used by the inbox query to
// filter out already-dismissed entries in a single round-trip.
func (r *Repository) ListDismissedInboxItemIDs(
	ctx context.Context, userID, kind string,
) ([]string, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT item_id FROM office_inbox_dismissals
		WHERE user_id = ? AND item_kind = ?
	`), userID, kind)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FailedRunInboxRow is a flat view of a failed run joined with
// its agent (so the inbox layer can render a row without a follow-up
// per-agent fetch). Excludes runs belonging to auto-paused agents
// since those are consolidated into the per-agent paused entry.
type FailedRunInboxRow struct {
	RunID          string `db:"run_id"`
	AgentProfileID string `db:"agent_profile_id"`
	AgentName      string `db:"agent_name"`
	WorkspaceID    string `db:"workspace_id"`
	TaskID         string `db:"task_id"`
	ErrorMessage   string `db:"error_message"`
	// FailedAt is COALESCE(finished_at, requested_at) — SQLite stores
	// these as TEXT so we scan into a string and parse on the Go side.
	FailedAtRaw string    `db:"failed_at"`
	FailedAt    time.Time `db:"-"`
}

// ListFailedRunsForInbox returns one row per failed run that:
//   - belongs to a workspace,
//   - whose agent is NOT currently auto-paused (those are covered by
//     the consolidated agent_paused_after_failures entry), and
//   - has no dismissal row from the given user OR the auto-dismiss
//     sentinel.
//
// Sorted by finished_at descending so the freshest failures float to
// the top of the inbox.
func (r *Repository) ListFailedRunsForInbox(
	ctx context.Context, workspaceID, userID string,
) ([]*FailedRunInboxRow, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT w.id AS run_id,
		       w.agent_profile_id,
		       a.name AS agent_name,
		       a.workspace_id,
		       COALESCE(json_extract(w.payload, '$.task_id'), '') AS task_id,
		       w.error_message,
		       COALESCE(w.finished_at, w.requested_at) AS failed_at
		FROM runs w
		JOIN agent_profiles a ON a.id = w.agent_profile_id
		WHERE w.status = 'failed'
		  AND a.workspace_id = ?
		  AND (a.pause_reason IS NULL OR a.pause_reason NOT LIKE 'Auto-paused:%')
		  AND NOT EXISTS (
		    SELECT 1 FROM office_inbox_dismissals d
		    WHERE d.item_kind = 'agent_run_failed'
		      AND d.item_id = w.id
		      AND d.user_id IN (?, '_auto')
		  )
		ORDER BY failed_at DESC
		LIMIT 200
	`), workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*FailedRunInboxRow
	for rows.Next() {
		var row FailedRunInboxRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		row.FailedAt = parseSqliteTime(row.FailedAtRaw)
		out = append(out, &row)
	}
	return out, rows.Err()
}

// parseSqliteTime parses the various string formats SQLite stores
// DATETIME values in (RFC3339, with or without timezone, with or
// without nanoseconds). Falls back to zero time on parse failure
// rather than erroring, since the inbox doesn't break if a row's
// timestamp is unparseable.
func parseSqliteTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// PausedAgentInboxRow is a flat view of an auto-paused agent with the
// snapshot of tasks affected (those whose most recent failed run
// is for this agent and isn't dismissed).
type PausedAgentInboxRow struct {
	AgentID             string    `db:"agent_id"`
	AgentName           string    `db:"agent_name"`
	WorkspaceID         string    `db:"workspace_id"`
	PauseReason         string    `db:"pause_reason"`
	UpdatedAt           time.Time `db:"updated_at"`
	ConsecutiveFailures int       `db:"consecutive_failures"`
}

// ListAutoPausedAgentsForInbox returns the agents auto-paused in the
// workspace whose paused-entry hasn't been user-dismissed.
func (r *Repository) ListAutoPausedAgentsForInbox(
	ctx context.Context, workspaceID, userID string,
) ([]*PausedAgentInboxRow, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT a.id AS agent_id,
		       a.name AS agent_name,
		       a.workspace_id,
		       a.pause_reason,
		       a.updated_at,
		       a.consecutive_failures
		FROM agent_profiles a
		WHERE a.workspace_id = ?
		  AND a.workspace_id != ''
		  AND a.deleted_at IS NULL
		  AND a.pause_reason LIKE 'Auto-paused:%'
		  AND NOT EXISTS (
		    SELECT 1 FROM office_inbox_dismissals d
		    WHERE d.item_kind = 'agent_paused_after_failures'
		      AND d.item_id = a.id
		      AND d.user_id IN (?, '_auto')
		  )
		ORDER BY a.updated_at DESC
		LIMIT 200
	`), workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*PausedAgentInboxRow
	for rows.Next() {
		var row PausedAgentInboxRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		out = append(out, &row)
	}
	return out, rows.Err()
}

// ListFailedRunsForAgent returns the runs for an agent whose
// status is `failed` and that haven't been dismissed by the user.
// Used by the FailureService when computing which prior per-task
// inbox entries to auto-dismiss on auto-pause.
func (r *Repository) ListFailedRunsForAgent(
	ctx context.Context, agentID string,
) ([]string, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id FROM runs
		WHERE agent_profile_id = ? AND status = 'failed'
		ORDER BY finished_at DESC
	`), agentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
