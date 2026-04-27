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

// ListCostEvents returns all cost events ordered by time.
func (r *Repository) ListCostEvents(ctx context.Context, _ string) ([]*models.CostEvent, error) {
	var events []*models.CostEvent
	err := r.ro.SelectContext(ctx, &events, `
		SELECT * FROM orchestrate_cost_events
		ORDER BY occurred_at DESC
	`)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []*models.CostEvent{}
	}
	return events, nil
}

// GetCostsByAgent returns aggregated costs grouped by agent.
func (r *Repository) GetCostsByAgent(ctx context.Context, _ string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, `
		SELECT agent_instance_id AS group_key,
			SUM(cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events
		GROUP BY agent_instance_id
	`)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByProject returns aggregated costs grouped by project.
func (r *Repository) GetCostsByProject(ctx context.Context, _ string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, `
		SELECT project_id AS group_key,
			SUM(cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events
		GROUP BY project_id
	`)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByModel returns aggregated costs grouped by model.
func (r *Repository) GetCostsByModel(ctx context.Context, _ string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, `
		SELECT model AS group_key,
			SUM(cost_cents) AS total_cents,
			COUNT(*) AS count
		FROM orchestrate_cost_events
		GROUP BY model
	`)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}
