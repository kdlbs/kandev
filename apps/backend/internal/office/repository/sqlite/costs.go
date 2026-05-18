package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CreateCostEvent records a new cost event.
func (r *Repository) CreateCostEvent(ctx context.Context, event *models.CostEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	event.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_cost_events (
			id, session_id, task_id, agent_profile_id, project_id,
			model, provider, tokens_in, tokens_cached_in, tokens_out,
			cost_subcents, estimated, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.SessionID, event.TaskID, event.AgentProfileID,
		event.ProjectID, event.Model, event.Provider, event.TokensIn,
		event.TokensCachedIn, event.TokensOut, event.CostSubcents,
		event.Estimated, event.OccurredAt, event.CreatedAt)
	return err
}

// ListCostEvents returns cost events filtered by workspace (via task join), ordered by time.
func (r *Repository) ListCostEvents(ctx context.Context, workspaceID string) ([]*models.CostEvent, error) {
	var events []*models.CostEvent
	err := r.ro.SelectContext(ctx, &events, r.ro.Rebind(`
		SELECT e.* FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?
		ORDER BY e.occurred_at DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []*models.CostEvent{}
	}
	return events, nil
}

// GetCostsByAgent returns aggregated costs grouped by agent, filtered by workspace.
// group_label resolves to the agent profile's display name; empty when the
// profile row has been deleted.
func (r *Repository) GetCostsByAgent(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT e.agent_profile_id AS group_key,
			COALESCE(MAX(NULLIF(ap.name, '')), '') AS group_label,
			SUM(e.cost_subcents) AS total_subcents,
			COUNT(*) AS count
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		LEFT JOIN agent_profiles ap ON ap.id = e.agent_profile_id
		WHERE t.workspace_id = ?
		GROUP BY e.agent_profile_id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByProject returns aggregated costs grouped by project, filtered by workspace.
// group_label resolves to the project name; empty when project_id is unset
// or the project row has been deleted.
func (r *Repository) GetCostsByProject(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT e.project_id AS group_key,
			COALESCE(MAX(NULLIF(op.name, '')), '') AS group_label,
			SUM(e.cost_subcents) AS total_subcents,
			COUNT(*) AS count
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		LEFT JOIN office_projects op ON op.id = e.project_id
		WHERE t.workspace_id = ?
		GROUP BY e.project_id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByModel returns aggregated costs grouped by (provider, model).
// The label prefixes the model with a friendly brand name derived from the
// provider column ("Claude - default", "OpenAI - gpt-5.4-mini") so a row
// like claude-acp's logical alias "default" is unambiguous. group_key uses
// "provider:model" to keep rows for the same model name under different
// providers distinct (e.g. anthropic:claude-sonnet-4 vs google:claude-sonnet-4
// — unlikely in practice but cheap to handle).
func (r *Repository) GetCostsByModel(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT
			COALESCE(NULLIF(e.provider, ''), '') || ':' || COALESCE(NULLIF(e.model, ''), '') AS group_key,
			CASE COALESCE(NULLIF(e.provider, ''), '')
				WHEN ''          THEN COALESCE(NULLIF(e.model, ''), '')
				WHEN 'anthropic' THEN 'Claude - '  || COALESCE(NULLIF(e.model, ''), '(unknown)')
				WHEN 'openai'    THEN 'OpenAI - '  || COALESCE(NULLIF(e.model, ''), '(unknown)')
				WHEN 'google'    THEN 'Gemini - '  || COALESCE(NULLIF(e.model, ''), '(unknown)')
				ELSE e.provider || ' - ' || COALESCE(NULLIF(e.model, ''), '(unknown)')
			END AS group_label,
			SUM(e.cost_subcents) AS total_subcents,
			COUNT(*) AS count
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?
		GROUP BY e.provider, e.model
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// GetCostsByProvider returns aggregated costs grouped by provider so users
// can see spend per vendor (Claude / OpenAI / Gemini). Provider is whatever
// the office cost subscriber resolved at write time — usually derived from
// the CLI id (claude-acp → anthropic, codex-acp → openai, ...). Empty
// provider values bucket under "(unknown)" so the row is still surfaced
// rather than silently dropped.
func (r *Repository) GetCostsByProvider(ctx context.Context, workspaceID string) ([]*models.CostBreakdown, error) {
	var results []*models.CostBreakdown
	err := r.ro.SelectContext(ctx, &results, r.ro.Rebind(`
		SELECT
			COALESCE(NULLIF(e.provider, ''), 'unknown') AS group_key,
			CASE COALESCE(NULLIF(e.provider, ''), 'unknown')
				WHEN 'anthropic' THEN 'Claude'
				WHEN 'openai'    THEN 'OpenAI'
				WHEN 'google'    THEN 'Gemini'
				WHEN 'unknown'   THEN '(unknown)'
				ELSE e.provider
			END AS group_label,
			SUM(e.cost_subcents) AS total_subcents,
			COUNT(*) AS count
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?
		GROUP BY COALESCE(NULLIF(e.provider, ''), 'unknown')
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []*models.CostBreakdown{}
	}
	return results, nil
}

// SumCosts returns the total cost in subcents (hundredths of a cent) for a
// workspace using SQL aggregation.
func (r *Repository) SumCosts(ctx context.Context, workspaceID string) (int64, error) {
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(e.cost_subcents), 0)
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?
	`), workspaceID).Scan(&total)
	return total, err
}

// SumCostsSince returns the workspace's total cost in subcents incurred at or
// after `since`. A zero `since` returns the lifetime total (same as SumCosts).
// Used by period-aware budget rollups.
func (r *Repository) SumCostsSince(ctx context.Context, workspaceID string, since time.Time) (int64, error) {
	if since.IsZero() {
		return r.SumCosts(ctx, workspaceID)
	}
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(e.cost_subcents), 0)
		FROM office_cost_events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ? AND e.occurred_at >= ?
	`), workspaceID, since.UTC()).Scan(&total)
	return total, err
}

// GetCostForAgent returns the total cost in subcents for a specific agent instance.
func (r *Repository) GetCostForAgent(ctx context.Context, agentID string) (int64, error) {
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(cost_subcents), 0)
		FROM office_cost_events
		WHERE agent_profile_id = ?
	`), agentID).Scan(&total)
	return total, err
}

// GetCostForAgentSince returns the agent's total cost in subcents incurred at
// or after `since`. Zero `since` is the lifetime total.
func (r *Repository) GetCostForAgentSince(ctx context.Context, agentID string, since time.Time) (int64, error) {
	if since.IsZero() {
		return r.GetCostForAgent(ctx, agentID)
	}
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(cost_subcents), 0)
		FROM office_cost_events
		WHERE agent_profile_id = ? AND occurred_at >= ?
	`), agentID, since.UTC()).Scan(&total)
	return total, err
}

// GetCostForProject returns the total cost in subcents for a specific project.
func (r *Repository) GetCostForProject(ctx context.Context, projectID string) (int64, error) {
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(cost_subcents), 0)
		FROM office_cost_events
		WHERE project_id = ?
	`), projectID).Scan(&total)
	return total, err
}

// GetCostForProjectSince returns the project's total cost in subcents
// incurred at or after `since`. Zero `since` is the lifetime total.
func (r *Repository) GetCostForProjectSince(ctx context.Context, projectID string, since time.Time) (int64, error) {
	if since.IsZero() {
		return r.GetCostForProject(ctx, projectID)
	}
	var total int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(cost_subcents), 0)
		FROM office_cost_events
		WHERE project_id = ? AND occurred_at >= ?
	`), projectID, since.UTC()).Scan(&total)
	return total, err
}
