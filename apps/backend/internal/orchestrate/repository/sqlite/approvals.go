package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateApproval creates a new approval request.
func (r *Repository) CreateApproval(ctx context.Context, approval *models.Approval) error {
	if approval.ID == "" {
		approval.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	approval.CreatedAt = now
	approval.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_approvals (
			id, workspace_id, type, requested_by_agent_instance_id, status,
			payload, decision_note, decided_by, decided_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), approval.ID, approval.WorkspaceID, approval.Type,
		approval.RequestedByAgentInstanceID, approval.Status, approval.Payload,
		approval.DecisionNote, approval.DecidedBy, approval.DecidedAt,
		approval.CreatedAt, approval.UpdatedAt)
	return err
}

// GetApproval returns an approval by ID.
func (r *Repository) GetApproval(ctx context.Context, id string) (*models.Approval, error) {
	var approval models.Approval
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_approvals WHERE id = ?`), id).StructScan(&approval)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("approval not found: %s", id)
	}
	return &approval, err
}

// ListApprovals returns all approvals for a workspace.
func (r *Repository) ListApprovals(ctx context.Context, workspaceID string) ([]*models.Approval, error) {
	var approvals []*models.Approval
	err := r.ro.SelectContext(ctx, &approvals, r.ro.Rebind(
		`SELECT * FROM orchestrate_approvals WHERE workspace_id = ? ORDER BY created_at DESC`), workspaceID)
	if err != nil {
		return nil, err
	}
	if approvals == nil {
		approvals = []*models.Approval{}
	}
	return approvals, nil
}

// ListPendingApprovals returns all pending approvals for a workspace.
func (r *Repository) ListPendingApprovals(ctx context.Context, workspaceID string) ([]*models.Approval, error) {
	var approvals []*models.Approval
	err := r.ro.SelectContext(ctx, &approvals, r.ro.Rebind(
		`SELECT * FROM orchestrate_approvals WHERE workspace_id = ? AND status = 'pending' ORDER BY created_at DESC`), workspaceID)
	if err != nil {
		return nil, err
	}
	if approvals == nil {
		approvals = []*models.Approval{}
	}
	return approvals, nil
}

// CountPendingApprovals returns the number of pending approvals for a workspace.
func (r *Repository) CountPendingApprovals(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := r.ro.GetContext(ctx, &count, r.ro.Rebind(
		`SELECT COUNT(*) FROM orchestrate_approvals WHERE workspace_id = ? AND status = 'pending'`), workspaceID)
	return count, err
}

// UpdateApproval updates an existing approval (used for deciding).
func (r *Repository) UpdateApproval(ctx context.Context, approval *models.Approval) error {
	approval.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_approvals SET
			status = ?, decision_note = ?, decided_by = ?, decided_at = ?, updated_at = ?
		WHERE id = ?
	`), approval.Status, approval.DecisionNote, approval.DecidedBy,
		approval.DecidedAt, approval.UpdatedAt, approval.ID)
	return err
}
