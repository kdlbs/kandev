package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateCostEvent records a new cost event.
func (r *Repository) CreateCostEvent(ctx context.Context, event *models.CostEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	event.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_cost_events (
			id, session_id, task_id, agent_instance_id, project_id,
			model, provider, tokens_in, tokens_cached_in, tokens_out,
			cost_cents, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.SessionID, event.TaskID, event.AgentInstanceID,
		event.ProjectID, event.Model, event.Provider, event.TokensIn,
		event.TokensCachedIn, event.TokensOut, event.CostCents,
		event.OccurredAt, event.CreatedAt)
	return err
}

// ListCostEvents returns cost events for a workspace within a time range.
func (r *Repository) ListCostEvents(ctx context.Context, workspaceID string) ([]*models.CostEvent, error) {
	var events []*models.CostEvent
	err := r.ro.SelectContext(ctx, &events, r.ro.Rebind(`
		SELECT ce.* FROM orchestrate_cost_events ce
		JOIN orchestrate_agent_instances ai ON ce.agent_instance_id = ai.id
		WHERE ai.workspace_id = ?
		ORDER BY ce.occurred_at DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []*models.CostEvent{}
	}
	return events, nil
}

// GetCostsByAgent returns aggregated costs grouped by agent.
func (r *Repository) GetCostsByAgent(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT ce.agent_instance_id AS group_key,
			SUM(ce.cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events ce
		JOIN orchestrate_agent_instances ai ON ce.agent_instance_id = ai.id
		WHERE ai.workspace_id = ?
		GROUP BY ce.agent_instance_id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByProject returns aggregated costs grouped by project.
func (r *Repository) GetCostsByProject(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT ce.project_id AS group_key,
			SUM(ce.cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events ce
		JOIN orchestrate_agent_instances ai ON ce.agent_instance_id = ai.id
		WHERE ai.workspace_id = ?
		GROUP BY ce.project_id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByModel returns aggregated costs grouped by model.
func (r *Repository) GetCostsByModel(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT ce.model AS group_key,
			SUM(ce.cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events ce
		JOIN orchestrate_agent_instances ai ON ce.agent_instance_id = ai.id
		WHERE ai.workspace_id = ?
		GROUP BY ce.model
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}
