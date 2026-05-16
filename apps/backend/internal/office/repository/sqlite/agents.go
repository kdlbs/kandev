// Office agent CRUD backed by the unified agent_profiles table
// (ADR 0005 Wave C; collapsed in Wave G). Office rows are agent_profiles
// rows with workspace_id != ''; shallow kanban profiles use
// workspace_id = '' and are invisible to these methods.
//
// After Wave G AgentInstance is a type alias for settings.AgentProfile,
// so the repo round-trips the canonical struct directly via sqlx
// StructScan. There is no longer a separate AgentProfileID field — the
// row's id IS the profile id.

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// agentInstanceColumns is the SELECT projection that maps agent_profiles
// columns onto the AgentInstance struct shape. Used by every read in this
// file so the column ordering is centralised.
//
// Wave G: every column listed here matches a `db:` tag on AgentProfile,
// so sqlx StructScan resolves the row directly. Columns the office shape
// does not surface (e.g. cli_flags, allow_indexing, plan, agent_id) are
// omitted; sqlx ignores struct fields that have no matching column.
const agentInstanceColumns = `
	id,
	COALESCE(agent_id, '')                  AS agent_id,
	COALESCE(workspace_id, '')              AS workspace_id,
	name,
	COALESCE(agent_display_name, '')        AS agent_display_name,
	COALESCE(model, '')                     AS model,
	COALESCE(allow_indexing, 0)             AS allow_indexing,
	COALESCE(cli_passthrough, 0)            AS cli_passthrough,
	COALESCE(role, '')                      AS role,
	COALESCE(icon, '')                      AS icon,
	COALESCE(status, 'idle')                AS status,
	COALESCE(reports_to, '')                AS reports_to,
	COALESCE(permissions, '{}')             AS permissions,
	COALESCE(budget_monthly_cents, 0)       AS budget_monthly_cents,
	COALESCE(max_concurrent_sessions, 1)    AS max_concurrent_sessions,
	COALESCE(cooldown_sec, 0)               AS cooldown_sec,
	COALESCE(skip_idle_runs, 0)             AS skip_idle_runs,
	last_run_finished_at,
	COALESCE(skill_ids, '[]')               AS skill_ids,
	COALESCE(desired_skills, '[]')          AS desired_skills,
	COALESCE(executor_preference, '')       AS executor_preference,
	COALESCE(pause_reason, '')              AS pause_reason,
	COALESCE(consecutive_failures, 0)       AS consecutive_failures,
	NULLIF(failure_threshold, 0)            AS failure_threshold,
	COALESCE(settings, '{}')                AS settings,
	created_at,
	updated_at
`

// agentInstanceFilter is the WHERE clause that scopes agent_profiles reads
// to office rows only (workspace_id != ” and not soft-deleted).
const agentInstanceFilter = `workspace_id != '' AND deleted_at IS NULL`

// CreateAgentInstance inserts a new office agent as an agent_profiles row.
// The struct's AgentProfileID is normalised to ID after insert.
func (r *Repository) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	if agent.WorkspaceID == "" {
		return fmt.Errorf("create agent instance: workspace_id is required")
	}
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	desiredSkills := normalizeAgentJSONArray(agent.DesiredSkills)
	skillIDs := normalizeAgentJSONArray(agent.SkillIDs)
	permissions := normalizeAgentJSONObject(agent.Permissions)
	threshold := failureThresholdToColumn(agent.FailureThreshold)
	status := string(agent.Status)
	if status == "" {
		status = "idle"
	}

	displayName := agent.AgentDisplayName
	if displayName == "" {
		displayName = agent.Name
	}
	// agent_profiles.agent_id is FK → agents (CLI tool registrations).
	// When the caller doesn't supply one (e.g. office's createAgent handler
	// receives only role + name), best-effort inherit from any existing
	// agent in the same workspace so the FK is satisfied. Falls back to
	// the first registered CLI tool. If neither yields a value (tests with
	// no CLI tools registered, or FK enforcement disabled), leave it empty
	// and let the underlying FK constraint speak for itself in production.
	if agent.AgentID == "" {
		var defaultAgentID string
		_ = r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
			SELECT agent_id FROM agent_profiles
			WHERE workspace_id = ? AND agent_id != '' AND deleted_at IS NULL
			ORDER BY created_at ASC LIMIT 1
		`), agent.WorkspaceID).Scan(&defaultAgentID)
		if defaultAgentID == "" {
			_ = r.ro.QueryRowxContext(ctx, r.ro.Rebind(
				`SELECT id FROM agents ORDER BY created_at ASC LIMIT 1`,
			)).Scan(&defaultAgentID)
		}
		agent.AgentID = defaultAgentID
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_profiles (
			id, agent_id, name, agent_display_name, model, mode,
			auto_approve, dangerously_skip_permissions, allow_indexing,
			cli_passthrough, user_modified, plan,
			created_at, updated_at,
			workspace_id, role, icon, reports_to,
			skill_ids, desired_skills, custom_prompt,
			status, pause_reason, last_run_finished_at,
			max_concurrent_sessions, cooldown_sec, skip_idle_runs,
			consecutive_failures, failure_threshold,
			executor_preference, budget_monthly_cents,
			settings, permissions
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, '',
			?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?,
			?, ?,
			'{}', ?
		)
	`),
		agent.ID, agent.AgentID, agent.Name, displayName, agent.Model, agent.Mode,
		boolToInt(agent.AutoApprove), boolToInt(agent.DangerouslySkipPermissions), boolToInt(agent.AllowIndexing),
		boolToInt(agent.CLIPassthrough), boolToInt(agent.UserModified),
		agent.CreatedAt, agent.UpdatedAt,
		agent.WorkspaceID, string(agent.Role), agent.Icon, agent.ReportsTo,
		skillIDs, desiredSkills, agent.CustomPrompt,
		status, agent.PauseReason, agent.LastRunFinishedAt,
		agent.MaxConcurrentSessions, agent.CooldownSec, boolToInt(agent.SkipIdleRuns),
		agent.ConsecutiveFailures, threshold,
		agent.ExecutorPreference, agent.BudgetMonthlyCents,
		permissions,
	)
	return err
}

// GetAgentInstance returns an agent instance by ID. Office-scoped: rows
// without a workspace_id are invisible.
func (r *Repository) GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error) {
	var agent models.AgentInstance
	query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE id = ? AND ` + agentInstanceFilter
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), id).StructScan(&agent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent instance not found: %s", id)
	}
	return &agent, err
}

// GetAgentInstanceByName returns an agent instance by workspace+name.
func (r *Repository) GetAgentInstanceByName(
	ctx context.Context, workspaceID, name string,
) (*models.AgentInstance, error) {
	var agent models.AgentInstance
	query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE workspace_id = ? AND name = ? AND deleted_at IS NULL`
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), workspaceID, name).StructScan(&agent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent instance not found: %s", name)
	}
	return &agent, err
}

// ListAgentInstances returns all agent instances for a workspace.
// An empty workspaceID returns rows from all workspaces (still office-only).
func (r *Repository) ListAgentInstances(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error) {
	var (
		agents []*models.AgentInstance
		err    error
	)
	if workspaceID == "" {
		query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE ` + agentInstanceFilter + ` ORDER BY created_at`
		err = r.ro.SelectContext(ctx, &agents, query)
	} else {
		query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at`
		err = r.ro.SelectContext(ctx, &agents, r.ro.Rebind(query), workspaceID)
	}
	if err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*models.AgentInstance{}
	}
	return agents, nil
}

// ListAgentInstancesByIDs returns the office agent_profiles rows whose ids
// are in `ids`, in unspecified order. Rows missing from the DB are omitted.
// An empty input returns an empty slice. Used by GetLiveRuns to enrich
// session rows with agent names without reading the entire workspace's
// agent list.
func (r *Repository) ListAgentInstancesByIDs(ctx context.Context, ids []string) ([]*models.AgentInstance, error) {
	if len(ids) == 0 {
		return []*models.AgentInstance{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT ` + agentInstanceColumns +
		` FROM agent_profiles WHERE id IN (` + strings.Join(placeholders, ",") + `) AND ` + agentInstanceFilter
	var agents []*models.AgentInstance
	if err := r.ro.SelectContext(ctx, &agents, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*models.AgentInstance{}
	}
	return agents, nil
}

// UpdateAgentInstance updates an existing agent instance.
func (r *Repository) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	agent.UpdatedAt = time.Now().UTC()
	desiredSkills := normalizeAgentJSONArray(agent.DesiredSkills)
	skillIDs := normalizeAgentJSONArray(agent.SkillIDs)
	permissions := normalizeAgentJSONObject(agent.Permissions)
	threshold := failureThresholdToColumn(agent.FailureThreshold)
	status := string(agent.Status)
	if status == "" {
		status = "idle"
	}
	// Settings is included so PATCH /agents/:id can persist routing
	// overrides written via applyRoutingOverride; empty string is
	// normalized to "{}" to keep the column non-null.
	settings := agent.Settings
	if settings == "" {
		settings = "{}"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles SET
			name = ?, role = ?, icon = ?, status = ?,
			reports_to = ?, permissions = ?, budget_monthly_cents = ?,
			max_concurrent_sessions = ?, cooldown_sec = ?, skip_idle_runs = ?,
			last_run_finished_at = ?,
			skill_ids = ?, desired_skills = ?, executor_preference = ?,
			pause_reason = ?, failure_threshold = ?, settings = ?, updated_at = ?
		WHERE id = ? AND `+agentInstanceFilter+`
	`), agent.Name, string(agent.Role), agent.Icon, status,
		agent.ReportsTo, permissions, agent.BudgetMonthlyCents,
		agent.MaxConcurrentSessions, agent.CooldownSec, boolToInt(agent.SkipIdleRuns),
		agent.LastRunFinishedAt,
		skillIDs, desiredSkills, agent.ExecutorPreference,
		agent.PauseReason, threshold, settings, agent.UpdatedAt, agent.ID)
	return err
}

// UpdateAgentSettings persists the agent_profiles.settings JSON blob for
// an agent. Used by onboarding to seed routing.tier_source / .provider_
// order_source markers on the freshly created CEO agent so the routing
// UI doesn't have to infer "inherit" from the absence of a key.
func (r *Repository) UpdateAgentSettings(
	ctx context.Context, id, settings string,
) error {
	if settings == "" {
		settings = "{}"
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET settings = ?, updated_at = ?
		WHERE id = ? AND `+agentInstanceFilter+`
	`), settings, now, id)
	return err
}

// UpdateAgentStatusFields persists status + pause_reason for an agent.
func (r *Repository) UpdateAgentStatusFields(
	ctx context.Context, id, status, pauseReason string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET status = ?, pause_reason = ?, updated_at = ?
		WHERE id = ? AND `+agentInstanceFilter+`
	`), status, pauseReason, now, id)
	return err
}

// GetAgentInstanceByNameAny returns the first agent instance matching a name
// across all workspaces. Used for ID-or-name lookups without workspace context.
func (r *Repository) GetAgentInstanceByNameAny(
	ctx context.Context, name string,
) (*models.AgentInstance, error) {
	var agent models.AgentInstance
	query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE name = ? AND ` + agentInstanceFilter + ` LIMIT 1`
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), name).StructScan(&agent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent instance not found: %s", name)
	}
	return &agent, err
}

// CountAgentInstancesByRole returns the number of agents with the given role,
// optionally excluding one agent by ID.
func (r *Repository) CountAgentInstancesByRole(
	ctx context.Context, workspaceID string, role string, excludeID string,
) (int, error) {
	conds := []string{agentInstanceFilter, "role = ?"}
	args := []interface{}{role}
	if workspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if excludeID != "" {
		conds = append(conds, "id != ?")
		args = append(args, excludeID)
	}
	query := `SELECT COUNT(*) FROM agent_profiles WHERE ` + strings.Join(conds, " AND ")
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(&count)
	return count, err
}

// AgentInstanceExistsByName checks whether an agent with the given name exists,
// optionally excluding one agent by ID.
func (r *Repository) AgentInstanceExistsByName(
	ctx context.Context, workspaceID, name, excludeID string,
) (bool, error) {
	conds := []string{agentInstanceFilter, "name = ?"}
	args := []interface{}{name}
	if workspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if excludeID != "" {
		conds = append(conds, "id != ?")
		args = append(args, excludeID)
	}
	query := `SELECT EXISTS(SELECT 1 FROM agent_profiles WHERE ` + strings.Join(conds, " AND ") + `)`
	var exists bool
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(&exists)
	return exists, err
}

// DeleteAgentInstance hard-deletes an agent instance by ID. The unified
// agent_profiles table uses soft-deletion via deleted_at — we set it here so
// office reads (which all filter `deleted_at IS NULL`) immediately stop
// returning the row, while preserving the audit trail.
func (r *Repository) DeleteAgentInstance(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE agent_profiles SET deleted_at = ?, updated_at = ? WHERE id = ?`), now, now, id)
	return err
}

// AgentListFilter specifies optional filters for listing agents.
type AgentListFilter struct {
	Role      string
	Status    string
	ReportsTo string
}

// ListAgentInstancesFiltered returns agent instances for a workspace matching filters.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListAgentInstancesFiltered(
	ctx context.Context, workspaceID string, filter AgentListFilter,
) ([]*models.AgentInstance, error) {
	conds := []string{agentInstanceFilter}
	var args []interface{}
	if workspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if filter.Role != "" {
		conds = append(conds, "role = ?")
		args = append(args, filter.Role)
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.ReportsTo != "" {
		conds = append(conds, "reports_to = ?")
		args = append(args, filter.ReportsTo)
	}
	query := `SELECT ` + agentInstanceColumns + ` FROM agent_profiles WHERE ` + strings.Join(conds, " AND ") + ` ORDER BY created_at`

	var agents []*models.AgentInstance
	err := r.ro.SelectContext(ctx, &agents, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*models.AgentInstance{}
	}
	return agents, nil
}

// normalizeAgentJSONArray returns "[]" for empty values; otherwise the input.
func normalizeAgentJSONArray(s string) string {
	if strings.TrimSpace(s) == "" {
		return "[]"
	}
	return s
}

// normalizeAgentJSONObject returns "{}" for empty values; otherwise the input.
func normalizeAgentJSONObject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

// failureThresholdToColumn maps the office *int (NULL = use workspace default)
// onto the merged column's INTEGER NOT NULL DEFAULT 3 semantics. We store
// 0 as "use workspace default" and round-trip it back to nil on read.
func failureThresholdToColumn(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// boolToInt is a local helper. We avoid pulling internal/db/dialect into
// every office call site for a one-line conversion.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
