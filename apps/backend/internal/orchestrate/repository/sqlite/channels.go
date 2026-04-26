package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
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
		INSERT INTO orchestrate_channels (
			id, workspace_id, agent_instance_id, platform, config,
			status, task_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), channel.ID, channel.WorkspaceID, channel.AgentInstanceID,
		channel.Platform, channel.Config, channel.Status, channel.TaskID,
		channel.CreatedAt, channel.UpdatedAt)
	return err
}

// GetChannel returns a channel by ID.
func (r *Repository) GetChannel(ctx context.Context, id string) (*models.Channel, error) {
	var channel models.Channel
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_channels WHERE id = ?`), id).StructScan(&channel)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	return &channel, err
}

// ListChannels returns all channels for a workspace.
func (r *Repository) ListChannels(ctx context.Context, workspaceID string) ([]*models.Channel, error) {
	var channels []*models.Channel
	err := r.ro.SelectContext(ctx, &channels, r.ro.Rebind(
		`SELECT * FROM orchestrate_channels WHERE workspace_id = ? ORDER BY created_at`), workspaceID)
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
		UPDATE orchestrate_channels SET
			platform = ?, config = ?, status = ?, task_id = ?, updated_at = ?
		WHERE id = ?
	`), channel.Platform, channel.Config, channel.Status, channel.TaskID,
		channel.UpdatedAt, channel.ID)
	return err
}

// DeleteChannel deletes a channel by ID.
func (r *Repository) DeleteChannel(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_channels WHERE id = ?`), id)
	return err
}
