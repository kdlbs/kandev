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
			idempotency_key, context_snapshot, requested_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), req.ID, req.AgentInstanceID, req.Reason, req.Payload, req.Status,
		req.CoalescedCount, req.IdempotencyKey, req.ContextSnapshot, req.RequestedAt)
	return err
}

// ListWakeupRequests returns wakeup requests for a workspace.
func (r *Repository) ListWakeupRequests(ctx context.Context, workspaceID string) ([]*models.WakeupRequest, error) {
	var reqs []*models.WakeupRequest
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT wq.* FROM orchestrate_wakeup_queue wq
		JOIN orchestrate_agent_instances ai ON wq.agent_instance_id = ai.id
		WHERE ai.workspace_id = ?
		ORDER BY wq.requested_at DESC
	`), workspaceID)
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
