package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateWakeupRequest creates a new wakeup queue entry.
func (r *Repository) CreateWakeupRequest(ctx context.Context, req *models.WakeupRequest) error {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	req.RequestedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_wakeup_queue (
			id, agent_instance_id, reason, payload, status, coalesced_count,
			idempotency_key, context_snapshot, retry_count, scheduled_retry_at, requested_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), req.ID, req.AgentInstanceID, req.Reason, req.Payload, req.Status,
		req.CoalescedCount, req.IdempotencyKey, req.ContextSnapshot,
		req.RetryCount, req.ScheduledRetryAt, req.RequestedAt)
	return err
}

// ListWakeupRequests returns all wakeup requests ordered by time.
func (r *Repository) ListWakeupRequests(ctx context.Context, _ string) ([]*models.WakeupRequest, error) {
	var reqs []*models.WakeupRequest
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT * FROM orchestrate_wakeup_queue
		ORDER BY requested_at DESC
	`))
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []*models.WakeupRequest{}
	}
	return reqs, nil
}

// ClaimWakeupRequest atomically claims the oldest queued wakeup for an agent.
func (r *Repository) ClaimWakeupRequest(ctx context.Context, agentInstanceID string) (*models.WakeupRequest, error) {
	now := time.Now().UTC()
	var req models.WakeupRequest
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue
		SET status = 'claimed', claimed_at = ?
		WHERE id = (
			SELECT id FROM orchestrate_wakeup_queue
			WHERE agent_instance_id = ? AND status = 'queued'
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

// FinishWakeupRequest marks a wakeup as finished.
func (r *Repository) FinishWakeupRequest(ctx context.Context, id, status string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue SET status = ?, finished_at = ? WHERE id = ?
	`), status, now, id)
	return err
}

// CheckIdempotencyKey returns true if the key already exists within the window.
func (r *Repository) CheckIdempotencyKey(ctx context.Context, key string, windowHours int) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM orchestrate_wakeup_queue
		WHERE idempotency_key = ? AND requested_at > ?
	`), key, cutoff).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CoalesceWakeup tries to merge with an existing queued wakeup for the same
// agent and reason within the given window. Returns true if coalesced.
func (r *Repository) CoalesceWakeup(
	ctx context.Context, agentInstanceID, reason string, windowSecs int, payload string,
) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowSecs) * time.Second)
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue
		SET coalesced_count = coalesced_count + 1, payload = ?
		WHERE id = (
			SELECT id FROM orchestrate_wakeup_queue
			WHERE agent_instance_id = ? AND reason = ? AND status = 'queued'
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

// ClaimNextEligibleWakeup atomically claims the next queued wakeup,
// skipping wakeups with a scheduled retry time in the future and
// agents that already have a claimed wakeup. Agent status and cooldown
// checks are performed in the service layer.
func (r *Repository) ClaimNextEligibleWakeup(ctx context.Context) (*models.WakeupRequest, error) {
	now := time.Now().UTC()
	var req models.WakeupRequest
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue
		SET status = 'claimed', claimed_at = ?
		WHERE id = (
			SELECT w.id FROM orchestrate_wakeup_queue w
			WHERE w.status = 'queued'
			  AND (
				SELECT COUNT(*) FROM orchestrate_wakeup_queue cw
				WHERE cw.agent_instance_id = w.agent_instance_id
				  AND cw.status = 'claimed'
			  ) = 0
			  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= ?)
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

// ScheduleRetry resets a wakeup to queued with an incremented retry count
// and a scheduled retry time.
func (r *Repository) ScheduleRetry(ctx context.Context, wakeupID string, retryAt time.Time, retryCount int) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue
		SET status = 'queued', retry_count = ?, scheduled_retry_at = ?,
		    claimed_at = NULL, finished_at = NULL
		WHERE id = ?
	`), retryCount, retryAt, wakeupID)
	return err
}

// CleanExpired deletes finished/failed wakeups older than the given time.
func (r *Repository) CleanExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM orchestrate_wakeup_queue
		WHERE status IN ('finished', 'failed') AND finished_at < ?
	`), olderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecoverStale resets claimed wakeups older than the given time back to queued.
func (r *Repository) RecoverStale(ctx context.Context, claimedOlderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_wakeup_queue
		SET status = 'queued', claimed_at = NULL
		WHERE status = 'claimed' AND claimed_at < ?
	`), claimedOlderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
