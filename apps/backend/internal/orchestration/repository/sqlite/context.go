package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) SaveContextPacket(ctx context.Context, p *models.ContextPacket, raw string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO orchestration_context_packets(id,binding_id,objective_id,profile_id,content_json)
	VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`), p.ID, p.BindingID, p.ObjectiveID, p.ProfileID, raw)
	return err
}

func (r *Repository) ContextPacket(ctx context.Context, binding, id string) (string, error) {
	var raw string
	err := r.db.GetContext(ctx, &raw, r.db.Rebind(`SELECT content_json FROM orchestration_context_packets WHERE binding_id=? AND id=?`), binding, id)
	return raw, err
}

// ContextBinding is internal dispatch lookup, not a public unscoped read API.
func (r *Repository) ContextBinding(ctx context.Context, id string) (*models.AssistantBinding, error) {
	var b models.AssistantBinding
	err := r.ro.GetContext(ctx, &b, r.ro.Rebind(`SELECT b.* FROM orchestration_assistant_bindings b
	JOIN orchestration_context_packets p ON p.binding_id=b.id
	JOIN workspace_orchestrators o ON o.agent_id=b.orchestrator_id AND o.workspace_id=b.workspace_id
	WHERE p.id=?`), id)
	return &b, err
}
