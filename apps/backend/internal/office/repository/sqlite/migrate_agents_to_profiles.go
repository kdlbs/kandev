package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// migrateOfficeAgentsToProfiles copies every office_agent_instances row into
// the merged agent_profiles table, using the instance id as the new profile
// id. The CLI fields (model, mode, agent_id, etc.) come from the legacy
// agent_profile_id the instance was configured against. Idempotent:
// agent_profiles rows with the same id are skipped, and the whole function
// short-circuits when the legacy table is absent.
//
// This is step 4 of ADR 0005 Wave A. Wave C drops the legacy
// office_agent_instances table immediately after this migration runs, so on
// post-drop databases the table-existence guard turns the function into a
// no-op.
func (r *Repository) migrateOfficeAgentsToProfiles() error {
	if !r.tableExists("office_agent_instances") {
		return nil
	}
	if !r.tableExists("agent_profiles") {
		return nil
	}
	instances, err := r.readAllInstancesForMigration()
	if err != nil {
		return err
	}

	var migrated int
	for _, inst := range instances {
		ok, err := r.insertProfileFromInstance(inst)
		if err != nil {
			return err
		}
		if ok {
			migrated++
		}
	}
	if migrated > 0 {
		fmt.Printf("office sqlite migrate agents->profiles: %d row(s) inserted\n", migrated)
	}
	return nil
}

// instanceMigrationRow is the subset of office_agent_instances needed to
// build a merged agent_profiles row.
type instanceMigrationRow struct {
	id                    string
	workspaceID           string
	name                  string
	agentProfileID        string
	role                  string
	icon                  string
	status                string
	reportsTo             string
	budgetMonthlyCents    int
	maxConcurrentSessions int
	cooldownSec           int
	skipIdleRuns          int
	lastRunFinishedAt     sql.NullTime
	desiredSkills         string
	executorPreference    string
	pauseReason           string
	consecutiveFailures   int
	failureThreshold      sql.NullInt64
	permissions           string
}

// readAllInstancesForMigration drains every office_agent_instances row into a
// slice. We materialise eagerly so the caller can issue per-row INSERT
// statements without holding the rows iterator open — SQLite's single-writer
// model would otherwise deadlock when the test suite runs against a
// MaxOpenConns(1) pool.
func (r *Repository) readAllInstancesForMigration() ([]instanceMigrationRow, error) {
	rows, err := r.db.Query(`SELECT
		id, workspace_id, name, agent_profile_id,
		role, icon, status, reports_to, budget_monthly_cents,
		max_concurrent_sessions, cooldown_sec, skip_idle_runs,
		last_run_finished_at, desired_skills, executor_preference,
		pause_reason, consecutive_failures, failure_threshold,
		COALESCE(permissions, '{}')
		FROM office_agent_instances`)
	if err != nil {
		return nil, fmt.Errorf("read office_agent_instances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []instanceMigrationRow
	for rows.Next() {
		var inst instanceMigrationRow
		if err := rows.Scan(
			&inst.id, &inst.workspaceID, &inst.name, &inst.agentProfileID,
			&inst.role, &inst.icon, &inst.status,
			&inst.reportsTo, &inst.budgetMonthlyCents, &inst.maxConcurrentSessions,
			&inst.cooldownSec, &inst.skipIdleRuns, &inst.lastRunFinishedAt,
			&inst.desiredSkills, &inst.executorPreference, &inst.pauseReason,
			&inst.consecutiveFailures, &inst.failureThreshold,
			&inst.permissions,
		); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return out, nil
}

// insertProfileFromInstance writes a new agent_profiles row using the
// instance's id. Returns ok=true when a row was inserted, ok=false when the
// row already existed (idempotent skip).
func (r *Repository) insertProfileFromInstance(inst instanceMigrationRow) (bool, error) {
	if r.profileExists(inst.id) {
		return false, nil
	}
	cli, ok, err := r.fetchProfileCLIFields(inst.agentProfileID)
	if err != nil {
		return false, fmt.Errorf("read source profile %q: %w", inst.agentProfileID, err)
	}
	if !ok {
		// Source profile missing — instance was orphaned. Skip silently rather
		// than failing the entire migration: the office row will be cleaned
		// up by Wave B's recreate-table migration.
		return false, nil
	}
	threshold := int64(3)
	if inst.failureThreshold.Valid {
		threshold = inst.failureThreshold.Int64
	}
	desired := inst.desiredSkills
	if desired == "" {
		desired = "[]"
	}
	permissions := inst.permissions
	if permissions == "" {
		permissions = "{}"
	}
	now := time.Now().UTC()
	_, err = r.db.Exec(`INSERT INTO agent_profiles (
		id, agent_id, name, agent_display_name, model, mode, migrated_from,
		auto_approve, dangerously_skip_permissions, allow_indexing,
		cli_passthrough, user_modified, plan, cli_flags,
		created_at, updated_at, deleted_at,
		workspace_id, role, icon, reports_to,
		skill_ids, desired_skills, custom_prompt,
		status, pause_reason, last_run_finished_at,
		max_concurrent_sessions, cooldown_sec, skip_idle_runs,
		consecutive_failures, failure_threshold,
		executor_preference, budget_monthly_cents, settings, permissions
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?,
		?, ?, '', ?,
		?, ?, NULL,
		?, ?, ?, ?,
		'[]', ?, '',
		?, ?, ?,
		?, ?, ?,
		?, ?,
		?, ?, '{}', ?
	)`,
		inst.id, cli.agentID, inst.name, cli.agentDisplayName, cli.model, cli.mode, "",
		cli.autoApprove, cli.skipPermissions, cli.allowIndexing,
		cli.cliPassthrough, 0, cli.cliFlags,
		now, now,
		inst.workspaceID, inst.role, inst.icon, inst.reportsTo,
		desired,
		inst.status, inst.pauseReason, inst.lastRunFinishedAt,
		inst.maxConcurrentSessions, inst.cooldownSec, inst.skipIdleRuns,
		inst.consecutiveFailures, threshold,
		inst.executorPreference, inst.budgetMonthlyCents,
		permissions,
	)
	if err != nil {
		return false, fmt.Errorf("insert profile %q: %w", inst.id, err)
	}
	return true, nil
}

// profileExists reports whether agent_profiles already has a row with the
// given id. Drives migration idempotency.
func (r *Repository) profileExists(id string) bool {
	var x int
	err := r.db.QueryRow(`SELECT 1 FROM agent_profiles WHERE id = ?`, id).Scan(&x)
	return err == nil && x == 1
}

// profileCLIFields holds the CLI-config columns we copy from the source
// shallow profile into the merged row.
type profileCLIFields struct {
	agentID          string
	agentDisplayName string
	model            string
	mode             sql.NullString
	autoApprove      int
	skipPermissions  int
	allowIndexing    int
	cliPassthrough   int
	cliFlags         sql.NullString
}

// fetchProfileCLIFields reads the CLI columns from the legacy shallow
// profile referenced by the office instance. Returns ok=false when the
// source profile is absent (orphaned instance) so the caller can skip it.
func (r *Repository) fetchProfileCLIFields(profileID string) (profileCLIFields, bool, error) {
	if profileID == "" {
		return profileCLIFields{}, false, nil
	}
	var cli profileCLIFields
	err := r.db.QueryRow(`SELECT
		agent_id, agent_display_name, model, mode,
		auto_approve, dangerously_skip_permissions, allow_indexing,
		cli_passthrough, cli_flags
		FROM agent_profiles WHERE id = ? AND deleted_at IS NULL`, profileID).Scan(
		&cli.agentID, &cli.agentDisplayName, &cli.model, &cli.mode,
		&cli.autoApprove, &cli.skipPermissions, &cli.allowIndexing,
		&cli.cliPassthrough, &cli.cliFlags,
	)
	if err == sql.ErrNoRows {
		return profileCLIFields{}, false, nil
	}
	if err != nil {
		return profileCLIFields{}, false, err
	}
	return cli, true, nil
}
