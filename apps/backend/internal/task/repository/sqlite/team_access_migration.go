package sqlite

// Team-access schema evolution: workspace org and unit placement, workspace
// membership, and the human task assignee.
//
// The governing rule for this migration is that an upgrade must never widen
// access. Every pre-existing workspace lands on 'private', which reproduces
// today's owner-only behavior exactly; opening a board to the organization is
// an explicit act by its owner afterwards.
func (r *Repository) ensureTeamAccessSchema() error {
	// New columns on existing tables arrive only through idempotent ADD COLUMN
	// (ADR 0027): CREATE TABLE IF NOT EXISTS is a no-op on an existing DB, so
	// listing them in the DDL alone would never reach an upgraded install.
	// Organizations. Empty means "not yet assigned"; the tenancy migration puts
	// every existing workspace into the default org on first boot with the
	// feature on.
	r.migrate.Apply(
		"workspaces.org_id",
		`ALTER TABLE workspaces ADD COLUMN org_id TEXT NOT NULL DEFAULT ''`,
	)
	r.migrate.Apply(
		"workspaces.org_idx",
		`CREATE INDEX IF NOT EXISTS idx_workspaces_org ON workspaces(org_id)`,
	)
	// The human assignee (docs/specs/tasks/requirements/human-assignee.md). Note that
	// internal/office's priority-to-TEXT migration recreates the tasks table
	// from a hardcoded column list on every database this repository creates,
	// so this column is ALSO listed there; adding it here alone leaves it
	// dropped by the time any task is written.
	r.migrate.Apply(
		"tasks.assignee_user_id",
		`ALTER TABLE tasks ADD COLUMN assignee_user_id TEXT NOT NULL DEFAULT ''`,
	)

	r.ensureOrgUnitPlacement()

	// workspace_members is created by the infra DDL on both fresh and existing
	// databases, so only the owner backfill needs to run here.
	return r.backfillWorkspaceOwnerMembers()
}

// backfillWorkspaceOwnerMembers writes the mirroring owner row for every
// workspace that has an owner. Workspaces created before authentication was
// enabled carry an empty owner_id and deliberately get no row: they stay
// visible to everyone until the setup wizard claims them, and inventing an
// owner here would lock the single user out of their own data.
func (r *Repository) backfillWorkspaceOwnerMembers() error {
	_, err := r.db.Exec(`
		INSERT INTO workspace_members (workspace_id, user_id, role, added_by, created_at)
		SELECT w.id, w.owner_id, 'owner', '', w.created_at
		FROM workspaces w
		WHERE w.owner_id IS NOT NULL
		  AND w.owner_id <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM workspace_members m
			WHERE m.workspace_id = w.id AND m.user_id = w.owner_id
		  )
	`)
	return err
}

// ensureOrgUnitPlacement adds the column that places a workspace in the
// organization unit tree.
//
// It lives here rather than in internal/orgunit because the task repository
// owns the workspaces table.
//
// No migration currently recreates `workspaces`. If one is ever added, this
// column has to be in its replacement CREATE TABLE and its INSERT ... SELECT
// list: a rebuild copies a fixed column list, and MigrateLogger.Apply swallows
// errors, so a column added before a rebuild disappears without a sound. That
// is recorded in apps/backend/AGENTS.md and has already cost this branch a
// silent loss once, on `tasks`.
func (r *Repository) ensureOrgUnitPlacement() {
	r.migrate.Apply(
		"workspaces.unit_id",
		`ALTER TABLE workspaces ADD COLUMN unit_id TEXT NOT NULL DEFAULT ''`,
	)
	r.migrate.Apply(
		"workspaces.unit_idx",
		`CREATE INDEX IF NOT EXISTS idx_workspaces_unit ON workspaces(unit_id)`,
	)
}
