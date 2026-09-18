package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	"time"
)

const runtimeIdle = "idle"

func (r *Repository) SetAssistantPaused(ctx context.Context, b *models.AssistantBinding, paused bool) error {
	state := runtimeIdle
	if paused {
		state = "paused"
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE agent_profiles SET status=?,updated_at=? WHERE id=?
 AND EXISTS(SELECT 1 FROM orchestration_assistant_bindings b WHERE b.id=? AND b.version=? AND b.orchestrator_id=agent_profiles.id)`), state, time.Now().UTC(), b.OrchestratorID, b.ID, b.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
func (r *Repository) AssistantManagedTasks(ctx context.Context, binding, after string, limit int) ([]string, error) {
	rows := []string{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT DISTINCT t.id FROM orchestration_assistant_bindings b
 JOIN orchestration_objectives o ON o.binding_id=b.id JOIN orchestration_objective_tasks l ON l.objective_id=o.id
 JOIN tasks t ON t.id=l.task_id WHERE b.id=? AND t.workspace_id=b.workspace_id AND t.id<>b.conversation_id AND t.id>?
 ORDER BY t.id LIMIT ?`), binding, after, min(max(limit, 1), 101))
	return rows, err
}

// A stop is fresh human intent. Invalidate older queued assistant commands
// before stopping workers, without fabricating a user chat message.
func (r *Repository) AdvanceAssistantControlIntent(ctx context.Context, b *models.AssistantBinding, expected int64) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO orchestration_conversation_intents(task_id,revision)
 SELECT conversation_id,? FROM orchestration_assistant_bindings WHERE id=? AND version=?
 AND COALESCE((SELECT revision FROM orchestration_conversation_intents WHERE task_id=?),0)=?
 ON CONFLICT(task_id) DO UPDATE SET revision=excluded.revision WHERE orchestration_conversation_intents.revision=?`), expected+1, b.ID, b.Version, b.ConversationID, expected, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
