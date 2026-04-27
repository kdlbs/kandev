package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// ListInstructions returns all instruction files for an agent instance.
func (r *Repository) ListInstructions(ctx context.Context, agentInstanceID string) ([]*models.InstructionFile, error) {
	var files []*models.InstructionFile
	err := r.ro.SelectContext(ctx, &files, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_instructions WHERE agent_instance_id = ? ORDER BY is_entry DESC, filename`),
		agentInstanceID)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []*models.InstructionFile{}
	}
	return files, nil
}

// GetInstruction returns a single instruction file by agent and filename.
func (r *Repository) GetInstruction(
	ctx context.Context, agentInstanceID, filename string,
) (*models.InstructionFile, error) {
	var f models.InstructionFile
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_instructions WHERE agent_instance_id = ? AND filename = ?`),
		agentInstanceID, filename).StructScan(&f)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("instruction file %q not found", filename)
		}
		return nil, err
	}
	return &f, nil
}

// UpsertInstruction creates or updates an instruction file.
func (r *Repository) UpsertInstruction(
	ctx context.Context, agentInstanceID, filename, content string, isEntry bool,
) error {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_agent_instructions (
			id, agent_instance_id, filename, content, is_entry, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_instance_id, filename) DO UPDATE SET
			content = excluded.content,
			is_entry = excluded.is_entry,
			updated_at = excluded.updated_at
	`), id, agentInstanceID, filename, content, isEntry, now, now)
	return err
}

// DeleteInstruction deletes an instruction file by agent and filename.
func (r *Repository) DeleteInstruction(ctx context.Context, agentInstanceID, filename string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_agent_instructions WHERE agent_instance_id = ? AND filename = ?`),
		agentInstanceID, filename)
	return err
}
