package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// UpsertAgentMemory creates or updates an agent memory entry.
func (r *Repository) UpsertAgentMemory(ctx context.Context, mem *models.AgentMemory) error {
	if mem.ID == "" {
		mem.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	mem.CreatedAt = now
	mem.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_agent_memory (
			id, agent_instance_id, layer, key, content, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_instance_id, layer, key) DO UPDATE SET
			content = excluded.content,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`), mem.ID, mem.AgentInstanceID, mem.Layer, mem.Key, mem.Content,
		mem.Metadata, mem.CreatedAt, mem.UpdatedAt)
	return err
}

// ListAgentMemory returns all memory entries for an agent.
func (r *Repository) ListAgentMemory(ctx context.Context, agentInstanceID string) ([]*models.AgentMemory, error) {
	var entries []*models.AgentMemory
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_memory WHERE agent_instance_id = ? ORDER BY layer, key`),
		agentInstanceID)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.AgentMemory{}
	}
	return entries, nil
}

// DeleteAgentMemory deletes a single memory entry by ID.
func (r *Repository) DeleteAgentMemory(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_agent_memory WHERE id = ?`), id)
	return err
}
