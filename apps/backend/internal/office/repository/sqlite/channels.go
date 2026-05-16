package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CreateChannel creates a new channel.
func (r *Repository) CreateChannel(ctx context.Context, channel *models.Channel) error {
	if channel.ID == "" {
		channel.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_channels (
			id, workspace_id, agent_profile_id, platform, config, webhook_secret,
			status, task_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), channel.ID, channel.WorkspaceID, channel.AgentProfileID,
		channel.Platform, channel.Config, channel.WebhookSecret, channel.Status, channel.TaskID,
		channel.CreatedAt, channel.UpdatedAt)
	return err
}

// GetChannel returns a channel by ID.
func (r *Repository) GetChannel(ctx context.Context, id string) (*models.Channel, error) {
	var channel models.Channel
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_channels WHERE id = ?`), id).StructScan(&channel)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	return &channel, err
}

// ListChannels returns all channels for a workspace.
func (r *Repository) ListChannels(ctx context.Context, workspaceID string) ([]*models.Channel, error) {
	var channels []*models.Channel
	err := r.ro.SelectContext(ctx, &channels, r.ro.Rebind(
		`SELECT * FROM office_channels WHERE workspace_id = ? ORDER BY created_at`), workspaceID)
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []*models.Channel{}
	}
	return channels, nil
}

// UpdateChannel updates an existing channel.
func (r *Repository) UpdateChannel(ctx context.Context, channel *models.Channel) error {
	channel.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_channels SET
			platform = ?, config = ?, webhook_secret = ?, status = ?, task_id = ?, updated_at = ?
		WHERE id = ?
	`), channel.Platform, channel.Config, channel.WebhookSecret, channel.Status, channel.TaskID,
		channel.UpdatedAt, channel.ID)
	return err
}

// ListChannelsByAgent returns all channels for an agent instance.
func (r *Repository) ListChannelsByAgent(ctx context.Context, agentInstanceID string) ([]*models.Channel, error) {
	var channels []*models.Channel
	err := r.ro.SelectContext(ctx, &channels, r.ro.Rebind(
		`SELECT * FROM office_channels WHERE agent_profile_id = ? ORDER BY created_at`),
		agentInstanceID)
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []*models.Channel{}
	}
	return channels, nil
}

// DeleteChannel deletes a channel by ID.
func (r *Repository) DeleteChannel(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_channels WHERE id = ?`), id)
	return err
}

// CreateChannelTask creates a long-lived task for a channel.
// Returns the task ID.
//
// ADR 0005 Wave F: tasks no longer carry an assignee column. Channel
// tasks are spawned without a workflow_step_id, so we cannot key a
// runner participant row to them; the assignee is already recorded on
// the office_channels row (agent_profile_id) which the channel handlers
// read directly. The argument is kept on the signature for callers
// that already pass it but the value is intentionally dropped here.
func (r *Repository) CreateChannelTask(ctx context.Context, workspaceID, title, _ string) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO tasks (
			id, workspace_id, title, state, created_at, updated_at
		) VALUES (?, ?, ?, 'IN_PROGRESS', ?, ?)
	`), id, workspaceID, title, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}
