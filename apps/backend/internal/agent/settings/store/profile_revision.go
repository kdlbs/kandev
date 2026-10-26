package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
)

// initProfileRevisions installs database-owned change tracking so every SQL
// writer invalidates snapshots, including writers outside the settings store.
func (r *sqliteRepository) initProfileRevisions() error {
	if _, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS agent_profile_revisions (
		profile_id TEXT PRIMARY KEY,
		revision BIGINT NOT NULL CHECK (revision > 0)
	)`); err != nil {
		return fmt.Errorf("create agent profile revisions: %w", err)
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := r.initPostgresProfileRevisionTriggers(); err != nil {
			return err
		}
	} else {
		if _, err := r.db.Exec(`DROP TRIGGER IF EXISTS agent_profile_revision_update`); err != nil {
			return fmt.Errorf("replace agent profile revision update trigger: %w", err)
		}
		for _, stmt := range []string{
			`CREATE TRIGGER IF NOT EXISTS agent_profile_revision_insert
			AFTER INSERT ON agent_profiles BEGIN
				INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
				ON CONFLICT(profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
			END`,
			`CREATE TRIGGER agent_profile_revision_update
			AFTER UPDATE ON agent_profiles
			WHEN ` + profileRevisionChangedSQLite + ` BEGIN
				INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
				ON CONFLICT(profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
			END`,
		} {
			if _, err := r.db.Exec(stmt); err != nil {
				return fmt.Errorf("create agent profile revision trigger: %w", err)
			}
		}
	}
	// Backfill after trigger installation: an intervening write wins and is
	// never reset to the legacy starting revision.
	if _, err := r.db.Exec(`INSERT INTO agent_profile_revisions (profile_id, revision)
		SELECT id, 1 FROM agent_profiles WHERE 1 = 1
		ON CONFLICT(profile_id) DO NOTHING`); err != nil {
		return fmt.Errorf("backfill agent profile revisions: %w", err)
	}
	return nil
}

func (r *sqliteRepository) initPostgresProfileRevisionTriggers() error {
	if _, err := r.db.Exec(`CREATE OR REPLACE FUNCTION bump_agent_profile_revision() RETURNS trigger AS $$
	BEGIN
		IF TG_OP = 'INSERT' THEN
			INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
			ON CONFLICT (profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
		ELSIF ROW(` + qualifiedProfileRevisionColumns("OLD") + `) IS DISTINCT FROM ROW(` + qualifiedProfileRevisionColumns("NEW") + `) THEN
			INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
			ON CONFLICT (profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`); err != nil {
		return fmt.Errorf("create agent profile revision function: %w", err)
	}
	for _, stmt := range []string{
		`CREATE OR REPLACE TRIGGER agent_profile_revision_insert
		AFTER INSERT ON agent_profiles FOR EACH ROW EXECUTE FUNCTION bump_agent_profile_revision()`,
		`CREATE OR REPLACE TRIGGER agent_profile_revision_update
		AFTER UPDATE ON agent_profiles FOR EACH ROW EXECUTE FUNCTION bump_agent_profile_revision()`,
	} {
		if _, err := r.db.Exec(stmt); err != nil {
			return fmt.Errorf("create agent profile revision trigger: %w", err)
		}
	}
	return nil
}

const profileRevisionColumns = `id, agent_id, name, agent_display_name, model, mode, migrated_from,
	auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, enabled, user_modified,
	plan, cli_flags, env_vars, deleted_at, workspace_id, role, icon, reports_to, skill_ids,
	desired_skills, custom_prompt, max_concurrent_sessions, cooldown_sec, skip_idle_runs,
	failure_threshold, executor_preference, execution_agent_profile_id, budget_monthly_cents,
	settings, permissions, command_prefix, fallback_model, auto_fallback, provider_kind,
	provider_base_url, provider_api_key_secret_id, require_exact_model, cursor_mcp_auth_enabled,
	cursor_plugins_mcp_enabled, mcp_selection_mode, mcp_selected_servers`

const profileRevisionChangedSQLite = `OLD.id IS NOT NEW.id OR OLD.agent_id IS NOT NEW.agent_id OR
	OLD.name IS NOT NEW.name OR OLD.agent_display_name IS NOT NEW.agent_display_name OR
	OLD.model IS NOT NEW.model OR OLD.mode IS NOT NEW.mode OR OLD.migrated_from IS NOT NEW.migrated_from OR
	OLD.auto_approve IS NOT NEW.auto_approve OR OLD.dangerously_skip_permissions IS NOT NEW.dangerously_skip_permissions OR
	OLD.allow_indexing IS NOT NEW.allow_indexing OR OLD.cli_passthrough IS NOT NEW.cli_passthrough OR
	OLD.enabled IS NOT NEW.enabled OR OLD.user_modified IS NOT NEW.user_modified OR OLD.plan IS NOT NEW.plan OR
	OLD.cli_flags IS NOT NEW.cli_flags OR OLD.env_vars IS NOT NEW.env_vars OR OLD.deleted_at IS NOT NEW.deleted_at OR
	OLD.workspace_id IS NOT NEW.workspace_id OR OLD.role IS NOT NEW.role OR OLD.icon IS NOT NEW.icon OR
	OLD.reports_to IS NOT NEW.reports_to OR OLD.skill_ids IS NOT NEW.skill_ids OR
	OLD.desired_skills IS NOT NEW.desired_skills OR OLD.custom_prompt IS NOT NEW.custom_prompt OR
	OLD.max_concurrent_sessions IS NOT NEW.max_concurrent_sessions OR OLD.cooldown_sec IS NOT NEW.cooldown_sec OR
	OLD.skip_idle_runs IS NOT NEW.skip_idle_runs OR OLD.failure_threshold IS NOT NEW.failure_threshold OR
	OLD.executor_preference IS NOT NEW.executor_preference OR
	OLD.execution_agent_profile_id IS NOT NEW.execution_agent_profile_id OR
	OLD.budget_monthly_cents IS NOT NEW.budget_monthly_cents OR OLD.settings IS NOT NEW.settings OR
	OLD.permissions IS NOT NEW.permissions OR OLD.command_prefix IS NOT NEW.command_prefix OR
	OLD.fallback_model IS NOT NEW.fallback_model OR OLD.auto_fallback IS NOT NEW.auto_fallback OR
	OLD.provider_kind IS NOT NEW.provider_kind OR OLD.provider_base_url IS NOT NEW.provider_base_url OR
	OLD.provider_api_key_secret_id IS NOT NEW.provider_api_key_secret_id OR
	OLD.require_exact_model IS NOT NEW.require_exact_model OR
	OLD.cursor_mcp_auth_enabled IS NOT NEW.cursor_mcp_auth_enabled OR
	OLD.cursor_plugins_mcp_enabled IS NOT NEW.cursor_plugins_mcp_enabled OR
	OLD.mcp_selection_mode IS NOT NEW.mcp_selection_mode OR
	OLD.mcp_selected_servers IS NOT NEW.mcp_selected_servers`

func qualifiedProfileRevisionColumns(alias string) string {
	columns := strings.Split(profileRevisionColumns, ",")
	for i := range columns {
		columns[i] = alias + "." + strings.TrimSpace(columns[i])
	}
	return strings.Join(columns, ", ")
}

// GetAgentProfileRevision returns the persisted mutation token for a live
// profile. Missing revision state is an error; callers must fail closed.
func (r *sqliteRepository) GetAgentProfileRevision(ctx context.Context, id string) (int64, error) {
	var revision int64
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT r.revision FROM agent_profile_revisions r
		JOIN agent_profiles p ON p.id = r.profile_id
		WHERE p.id = ? AND p.deleted_at IS NULL`), id).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("read agent profile revision: %w", err)
	}
	return revision, nil
}
