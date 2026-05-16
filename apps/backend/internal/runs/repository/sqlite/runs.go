package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CreateRun creates a new run queue entry.
func (r *Repository) CreateRun(ctx context.Context, req *models.Run) error {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	ensureRunDefaults(req)
	req.RequestedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			idempotency_key, context_snapshot, capabilities, input_snapshot,
			output_summary, failure_reason, session_id, retry_count, scheduled_retry_at,
			requested_at, error_message, cancel_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), req.ID, req.AgentProfileID, req.Reason, req.Payload, req.Status,
		req.CoalescedCount, req.IdempotencyKey, req.ContextSnapshot,
		req.Capabilities, req.InputSnapshot, req.OutputSummary, req.FailureReason,
		req.SessionID, req.RetryCount, req.ScheduledRetryAt, req.RequestedAt,
		req.ErrorMessage, req.CancelReason)
	return err
}

func ensureRunDefaults(req *models.Run) {
	if req.Payload == "" {
		req.Payload = "{}"
	}
	if req.ContextSnapshot == "" {
		req.ContextSnapshot = "{}"
	}
	if req.Capabilities == "" {
		req.Capabilities = "{}"
	}
	if req.InputSnapshot == "" {
		req.InputSnapshot = "{}"
	}
	if req.ResultJSON == "" {
		req.ResultJSON = "{}"
	}
	if req.Status == "" {
		req.Status = "queued"
	}
}

// UpdateRunRuntimeSnapshot stores the runtime context captured before launch.
func (r *Repository) UpdateRunRuntimeSnapshot(
	ctx context.Context,
	id string,
	capabilities string,
	inputSnapshot string,
	sessionID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET capabilities = ?, input_snapshot = ?, session_id = ?
		WHERE id = ?
	`), capabilities, inputSnapshot, sessionID, id)
	return err
}

// UpdateRunPromptArtifacts persists the assembled prompt the agent
// received and the continuation-summary content prepended at dispatch.
// Called from the scheduler-integration after BuildAgentPrompt completes
// so the run-detail UI can render exactly what the agent saw. Either
// argument may be empty — the columns default to ” and tolerate it.
func (r *Repository) UpdateRunPromptArtifacts(
	ctx context.Context, id, assembledPrompt, summaryInjected string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET assembled_prompt = ?, summary_injected = ?
		WHERE id = ?
	`), assembledPrompt, summaryInjected, id)
	return err
}

// UpdateRunResultJSON stores the structured adapter output captured
// at run completion. Used by the continuation-summary builder as the
// primary input for "Recent actions" / "Recent decisions". Falls back
// to '{}' when raw is empty so the column default invariant holds.
func (r *Repository) UpdateRunResultJSON(ctx context.Context, id, raw string) error {
	if raw == "" {
		raw = "{}"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET result_json = ?
		WHERE id = ?
	`), raw, id)
	return err
}

// UpdateRunOutputSummary stores the final runtime summary for a run.
func (r *Repository) UpdateRunOutputSummary(ctx context.Context, id, outputSummary, failureReason string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET output_summary = ?, failure_reason = ?
		WHERE id = ?
	`), outputSummary, failureReason, id)
	return err
}

// ListRuns returns run requests filtered by workspace (via agent profile join), ordered by time.
func (r *Repository) ListRuns(ctx context.Context, workspaceID string) ([]*models.Run, error) {
	var reqs []*models.Run
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT w.* FROM runs w
		JOIN agent_profiles a ON a.id = w.agent_profile_id
		WHERE a.workspace_id = ?
		ORDER BY w.requested_at DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []*models.Run{}
	}
	return reqs, nil
}

// ClaimRun atomically claims the oldest queued run for an agent.
func (r *Repository) ClaimRun(ctx context.Context, agentInstanceID string) (*models.Run, error) {
	now := time.Now().UTC()
	var req models.Run
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'claimed', claimed_at = ?
		WHERE id = (
			SELECT id FROM runs
			WHERE agent_profile_id = ? AND status = 'queued'
			ORDER BY requested_at ASC
			LIMIT 1
		)
		RETURNING *
	`), now, agentInstanceID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// FinishRun marks a run as finished.
func (r *Repository) FinishRun(ctx context.Context, id, status string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET status = ?, finished_at = ? WHERE id = ?
	`), status, now, id)
	return err
}

// GetRunByID returns the run row for a given ID. Returns sql.ErrNoRows when unknown.
func (r *Repository) GetRunByID(ctx context.Context, id string) (*models.Run, error) {
	var run models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs WHERE id = ?
	`), id).StructScan(&run)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// CommentRunStatus is the slim per-comment run snapshot returned by
// GetRunsByCommentIDs. The backend maps these onto CommentDTO so the
// frontend can render a Queued / Working / Failed badge on the user
// comment that triggered each run.
type CommentRunStatus struct {
	RunID        string
	Status       string
	ErrorMessage string
}

// GetRunsByCommentIDs returns the latest run associated with each
// comment id by joining on idempotency_key = "task_comment:<comment_id>".
// Comments without a matching run are simply absent from the map.
// When a comment has multiple matching rows (rare — would require the
// idempotency window to lapse and the comment to re-trigger) the most
// recently requested row wins.
func (r *Repository) GetRunsByCommentIDs(
	ctx context.Context, commentIDs []string,
) (map[string]CommentRunStatus, error) {
	out := map[string]CommentRunStatus{}
	if len(commentIDs) == 0 {
		return out, nil
	}
	keys := make([]string, len(commentIDs))
	args := make([]interface{}, len(commentIDs))
	placeholders := make([]string, len(commentIDs))
	for i, id := range commentIDs {
		keys[i] = "task_comment:" + id
		args[i] = keys[i]
		placeholders[i] = "?"
	}
	query := fmt.Sprintf(`
		SELECT id, idempotency_key, status, error_message, requested_at
		FROM runs
		WHERE idempotency_key IN (%s)
		ORDER BY requested_at DESC
	`, strings.Join(placeholders, ","))
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id, idemKey, status, errMsg string
			requestedAt                 time.Time
		)
		if err := rows.Scan(&id, &idemKey, &status, &errMsg, &requestedAt); err != nil {
			return nil, err
		}
		commentID := strings.TrimPrefix(idemKey, "task_comment:")
		// First write wins — rows are ordered DESC so the first row per
		// comment id is the most recent.
		if _, ok := out[commentID]; ok {
			continue
		}
		out[commentID] = CommentRunStatus{
			RunID:        id,
			Status:       status,
			ErrorMessage: errMsg,
		}
	}
	return out, rows.Err()
}

// GetClaimedTasklessRunForAgent returns the most recently claimed
// taskless run (payload.task_id is empty / missing) for the given
// agent. Used by the AgentCompleted event subscriber to attribute a
// fresh-session heartbeat run when no task_id is on the event. Returns
// sql.ErrNoRows when no such run exists — the caller should treat
// that as "not a heartbeat completion event".
func (r *Repository) GetClaimedTasklessRunForAgent(
	ctx context.Context, agentProfileID string,
) (*models.Run, error) {
	var req models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status = 'claimed'
		  AND COALESCE(json_extract(payload, '$.task_id'), '') = ''
		ORDER BY claimed_at DESC
		LIMIT 1
	`), agentProfileID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// GetClaimedRunByTaskID returns the claimed run associated with a task payload.
func (r *Repository) GetClaimedRunByTaskID(ctx context.Context, taskID string) (*models.Run, error) {
	var req models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE status = 'claimed'
		  AND json_extract(payload, '$.task_id') = ?
		ORDER BY claimed_at DESC
		LIMIT 1
	`), taskID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// CheckIdempotencyKey returns true if the key already exists within the window.
func (r *Repository) CheckIdempotencyKey(ctx context.Context, key string, windowHours int) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE idempotency_key = ? AND requested_at > ?
	`), key, cutoff).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CoalesceRun tries to merge with an existing queued run for the same
// agent and reason within the given window. Returns true if coalesced.
func (r *Repository) CoalesceRun(
	ctx context.Context, agentInstanceID, reason string, windowSecs int, payload string,
) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowSecs) * time.Second)
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET coalesced_count = coalesced_count + 1, payload = ?
		WHERE id = (
			SELECT id FROM runs
			WHERE agent_profile_id = ? AND reason = ? AND status = 'queued'
			  AND requested_at > ?
			ORDER BY requested_at DESC
			LIMIT 1
		)
	`), payload, agentInstanceID, reason, cutoff)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ClaimNextEligibleRun atomically claims the next queued run,
// skipping runs with a scheduled retry time in the future and
// agents that already have a claimed run. Agent status and cooldown
// checks are performed in the service layer.
func (r *Repository) ClaimNextEligibleRun(ctx context.Context) (*models.Run, error) {
	now := time.Now().UTC()
	var req models.Run
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'claimed', claimed_at = ?
		WHERE id = (
			SELECT w.id FROM runs w
			WHERE w.status = 'queued'
			  AND (
				SELECT COUNT(*) FROM runs cw
				WHERE cw.agent_profile_id = w.agent_profile_id
				  AND cw.status = 'claimed'
			  ) = 0
			  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= ?)
			  AND w.routing_blocked_status IS NULL
			ORDER BY w.requested_at ASC
			LIMIT 1
		)
		RETURNING *
	`), now, now).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// ScheduleRetry resets a run to queued with an incremented retry count
// and a scheduled retry time.
func (r *Repository) ScheduleRetry(ctx context.Context, runID string, retryAt time.Time, retryCount int) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', retry_count = ?, scheduled_retry_at = ?,
		    claimed_at = NULL, finished_at = NULL
		WHERE id = ?
	`), retryCount, retryAt, runID)
	return err
}

// CleanExpired deletes finished/failed runs older than the given time.
func (r *Repository) CleanExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM runs
		WHERE status IN ('finished', 'failed') AND finished_at < ?
	`), olderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecoverStale resets claimed runs older than the given time back to queued.
func (r *Repository) RecoverStale(ctx context.Context, claimedOlderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', claimed_at = NULL
		WHERE status = 'claimed' AND claimed_at < ?
	`), claimedOlderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CancelRun marks a run as cancelled with an optional cancel reason.
func (r *Repository) CancelRun(ctx context.Context, id, cancelReason string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'cancelled', cancel_reason = ?, finished_at = ?
		WHERE id = ?
	`), cancelReason, now, id)
	return err
}

// ListPendingRunsForTask returns queued runs in retry state for the given task.
func (r *Repository) ListPendingRunsForTask(ctx context.Context, taskID string) ([]*models.Run, error) {
	var reqs []*models.Run
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE status = 'queued'
		  AND scheduled_retry_at IS NOT NULL
		  AND json_extract(payload, '$.task_id') = ?
	`), taskID)
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []*models.Run{}
	}
	return reqs, nil
}

// FindInflightRunForAgent returns the most-recent in-flight run for
// the given agent. "In-flight" means status='queued' (with or without a
// scheduled retry) or status='claimed'. The dispatcher uses this for
// claim-time coalescing — when a wakeup-request lands and an in-flight
// run already exists for the agent, the new request is merged into the
// existing run rather than creating a fresh one.
//
// Returns sql.ErrNoRows when no such run exists.
func (r *Repository) FindInflightRunForAgent(
	ctx context.Context, agentProfileID string,
) (*models.Run, error) {
	var run models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status IN ('queued', 'claimed')
		ORDER BY requested_at DESC
		LIMIT 1
	`), agentProfileID).StructScan(&run)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ListRunsForAgentPaged returns runs for an agent ordered by
// (requested_at DESC, id DESC), starting strictly after the given
// cursor. Pass cursor.IsZero() == true to fetch the first page.
// Returns at most `limit` rows; tie-break on id keeps adjacent rows
// with identical requested_at strictly ordered. The handler is
// expected to derive next_cursor from the last row's requested_at.
func (r *Repository) ListRunsForAgentPaged(
	ctx context.Context, agentInstanceID string, cursor time.Time, cursorID string, limit int,
) ([]*models.Run, error) {
	if limit <= 0 {
		limit = 25
	}
	var reqs []*models.Run
	if cursor.IsZero() {
		err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
			SELECT * FROM runs
			WHERE agent_profile_id = ?
			ORDER BY requested_at DESC, id DESC
			LIMIT ?
		`), agentInstanceID, limit)
		if err != nil {
			return nil, err
		}
	} else {
		// Strictly after the cursor (requested_at, id) DESC: a row is
		// "after" when its requested_at < cursor, OR requested_at ==
		// cursor AND id < cursorID.
		err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
			SELECT * FROM runs
			WHERE agent_profile_id = ?
			  AND (requested_at < ? OR (requested_at = ? AND id < ?))
			ORDER BY requested_at DESC, id DESC
			LIMIT ?
		`), agentInstanceID, cursor, cursor, cursorID, limit)
		if err != nil {
			return nil, err
		}
	}
	if reqs == nil {
		reqs = []*models.Run{}
	}
	return reqs, nil
}

// RunCostRollup carries the per-run aggregated token + cost numbers
// returned by GetRunWithCosts. Cost rows are joined to the run via
// the run payload's task_id; if the run has no cost rows yet the
// fields are zero. CostSubcents stores hundredths of a cent (UI divides
// by 10000).
type RunCostRollup struct {
	InputTokens  int64 `db:"input_tokens" json:"input_tokens"`
	OutputTokens int64 `db:"output_tokens" json:"output_tokens"`
	CachedTokens int64 `db:"cached_tokens" json:"cached_tokens"`
	CostSubcents int64 `db:"cost_subcents" json:"cost_subcents"`
}

// GetRunWithCosts returns the run row plus a token + cost rollup
// computed by joining office_cost_events on the task_id stored in
// the run payload (json_extract). Returns sql.ErrNoRows when the run
// id is unknown. The rollup may be all-zero when the run hasn't
// produced cost events yet.
func (r *Repository) GetRunWithCosts(
	ctx context.Context, runID string,
) (*models.Run, *RunCostRollup, error) {
	var run models.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs WHERE id = ?
	`), runID).StructScan(&run)
	if err != nil {
		return nil, nil, err
	}
	var rollup RunCostRollup
	err = r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COALESCE(SUM(tokens_in), 0)        AS input_tokens,
			COALESCE(SUM(tokens_out), 0)       AS output_tokens,
			COALESCE(SUM(tokens_cached_in), 0) AS cached_tokens,
			COALESCE(SUM(cost_subcents), 0)    AS cost_subcents
		FROM office_cost_events
		WHERE task_id != ''
		  AND task_id = COALESCE(json_extract(?, '$.task_id'), '')
	`), run.Payload).StructScan(&rollup)
	if err != nil {
		return &run, &RunCostRollup{}, nil
	}
	return &run, &rollup, nil
}

// SetRunRequestedAtForTest backfills requested_at for a seeded run.
// Test-only: bypasses the normal CreateRun stamping so E2E specs can
// build deterministic ordered pages without sleeping between writes.
func (r *Repository) SetRunRequestedAtForTest(
	ctx context.Context, runID string, ts time.Time,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET requested_at = ? WHERE id = ?
	`), ts, runID)
	return err
}

// SetRunStatusForTest forces the status + timing fields for a seeded
// run. Test-only: lets the E2E harness land non-queued rows
// (claimed/finished/failed/cancelled) without going through the
// production state machine.
func (r *Repository) SetRunStatusForTest(
	ctx context.Context, runID, status string,
	claimedAt, finishedAt *time.Time,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = ?, claimed_at = ?, finished_at = ?
		WHERE id = ?
	`), status, claimedAt, finishedAt, runID)
	return err
}

// SetRunErrorMessageForTest forces the error_message field on a run row.
// Test-only: lets the harness simulate a failed run carrying the same
// error string a real agent_failed event would have produced.
func (r *Repository) SetRunErrorMessageForTest(
	ctx context.Context, runID, errMsg string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET error_message = ? WHERE id = ?
	`), errMsg, runID)
	return err
}

// BulkCancelRuns cancels multiple runs by ID with the given reason.
func (r *Repository) BulkCancelRuns(ctx context.Context, ids []string, cancelReason string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+2)
	args = append(args, cancelReason, now)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`UPDATE runs SET status = 'cancelled', cancel_reason = ?, finished_at = ? WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}
