package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// UpsertAgentMemory creates or updates an agent memory entry.
func (r *Repository) UpsertAgentMemory(ctx context.Context, mem *models.AgentMemory) error {
	mem.Content = redaction.NewRedactor().String(mem.Content)
	if mem.ID == "" {
		mem.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	mem.CreatedAt = now
	mem.UpdatedAt = now

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestration_memory (
			id, agent_profile_id, layer, key, content, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_profile_id, layer, key) DO UPDATE SET
			content = excluded.content,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at, revision = orchestration_memory.revision+1
		WHERE orchestration_memory.confirmed=0 AND orchestration_memory.owner_user_id='' AND orchestration_memory.forgotten_at IS NULL
	`), mem.ID, mem.AgentProfileID, mem.Layer, mem.Key, mem.Content,
		mem.Metadata, mem.CreatedAt, mem.UpdatedAt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return models.ErrConflict
	}
	return err
}

// ListAgentMemory returns all memory entries for an agent.
func (r *Repository) ListAgentMemory(ctx context.Context, agentInstanceID string) ([]*models.AgentMemory, error) {
	var entries []*models.AgentMemory
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM orchestration_memory WHERE agent_profile_id = ? AND forgotten_at IS NULL AND (expires_at IS NULL OR expires_at>CURRENT_TIMESTAMP) ORDER BY layer, key`),
		agentInstanceID)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.AgentMemory{}
	}
	return entries, nil
}

// GetAgentMemory returns a single memory entry by agent, layer, and key.
func (r *Repository) GetAgentMemory(ctx context.Context, agentInstanceID, layer, key string) (*models.AgentMemory, error) {
	var mem models.AgentMemory
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestration_memory WHERE agent_profile_id = ? AND layer = ? AND key = ? AND forgotten_at IS NULL AND (expires_at IS NULL OR expires_at>CURRENT_TIMESTAMP)`),
		agentInstanceID, layer, key).StructScan(&mem)
	if err != nil {
		return nil, err
	}
	return &mem, nil
}

// ListAgentMemoryByLayer returns memory entries for an agent filtered by layer.
func (r *Repository) ListAgentMemoryByLayer(ctx context.Context, agentInstanceID, layer string) ([]*models.AgentMemory, error) {
	var entries []*models.AgentMemory
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM orchestration_memory WHERE agent_profile_id = ? AND layer = ? AND forgotten_at IS NULL AND (expires_at IS NULL OR expires_at>CURRENT_TIMESTAMP) ORDER BY key`),
		agentInstanceID, layer)
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
		`DELETE FROM orchestration_memory WHERE id = ?`), id)
	return err
}

// DeleteAgentMemoryOwned deletes a single memory entry only if it belongs to
// the given agent instance, preventing cross-agent deletion.
func (r *Repository) DeleteAgentMemoryOwned(ctx context.Context, agentInstanceID, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestration_memory WHERE id = ? AND agent_profile_id = ?`),
		id, agentInstanceID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("memory entry not found or does not belong to agent %s", agentInstanceID)
	}
	return nil
}

// DeleteAllAgentMemory deletes all memory entries for an agent.
func (r *Repository) DeleteAllAgentMemory(ctx context.Context, agentInstanceID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestration_memory WHERE agent_profile_id = ?`), agentInstanceID)
	return err
}

// MemoryContext selects generally applicable context. The runtime applies a
// byte budget and fails closed if indispensable confirmed rules cannot fit.
func (r *Repository) MemoryContext(ctx context.Context, id string) ([]*models.AgentMemory, error) {
	rows := []*models.AgentMemory{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT * FROM orchestration_memory WHERE agent_profile_id=?
	AND forgotten_at IS NULL AND (expires_at IS NULL OR expires_at>CURRENT_TIMESTAMP) AND scope IN ('user','workspace')
	ORDER BY confirmed DESC,priority DESC,updated_at DESC,id`), id)
	return rows, err
}
