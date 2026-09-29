package coordinator

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// phase2ColumnMigrations add the control-surface columns to the phase-1
// tables. They run after the phase-1 tables exist and never appear in
// createTablesSQL, so an existing database gains them by ALTER TABLE.
var phase2ColumnMigrations = []struct{ name, stmt string }{
	{"coordinators.policy_json", `ALTER TABLE coordinators ADD COLUMN policy_json TEXT`},
	{"coordinators.policy_revision", `ALTER TABLE coordinators ADD COLUMN policy_revision INTEGER NOT NULL DEFAULT 0`},
	{"coordinators.watch_scope", `ALTER TABLE coordinators ADD COLUMN watch_scope TEXT NOT NULL DEFAULT 'all'`},
	{"coordinator_proposals.kind", `ALTER TABLE coordinator_proposals ADD COLUMN kind TEXT NOT NULL DEFAULT 'create_task'`},
	{"coordinator_proposals.target_task_id", `ALTER TABLE coordinator_proposals ADD COLUMN target_task_id TEXT`},
	{"coordinator_proposals.standing_order_ids", `ALTER TABLE coordinator_proposals ADD COLUMN standing_order_ids TEXT NOT NULL DEFAULT '[]'`},
	{"coordinator_proposals.starts_agent", `ALTER TABLE coordinator_proposals ADD COLUMN starts_agent {{boolean}} NOT NULL DEFAULT FALSE`},
	{"coordinator_proposals.outcome_json", `ALTER TABLE coordinator_proposals ADD COLUMN outcome_json TEXT`},
}

// phase2TablesSQL creates the phase-2 tables. Indexes over columns added by
// phase2ColumnMigrations live in phase2IndexesSQL, which runs afterwards.
const phase2TablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_watches (
		coordinator_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		created_at {{timestamp}} NOT NULL,
		PRIMARY KEY (coordinator_id, workflow_id)
	);

	CREATE TABLE IF NOT EXISTS coordinator_activity (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		action_class TEXT NOT NULL,
		outcome TEXT NOT NULL,
		"authorization" TEXT NOT NULL,
		target_task_id TEXT,
		proposal_id TEXT,
		actor_user_id TEXT,
		reason_code TEXT,
		detail TEXT NOT NULL DEFAULT '',
		refusal_count INTEGER NOT NULL DEFAULT 1,
		edited {{boolean}} NOT NULL DEFAULT FALSE,
		undone_at {{timestamp}},
		undone_by TEXT,
		undo_of_id TEXT,
		created_at {{timestamp}} NOT NULL,
		updated_at {{timestamp}} NOT NULL
	);

	CREATE TABLE IF NOT EXISTS coordinator_standing_orders (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		text TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at {{timestamp}} NOT NULL,
		retired_at {{timestamp}},
		retired_by TEXT,
		source_proposal_id TEXT,
		last_applied_at {{timestamp}}
	);

	CREATE TABLE IF NOT EXISTS coordinator_goals (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		due_on TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		criteria_json TEXT NOT NULL DEFAULT '[]',
		baseline_json TEXT NOT NULL DEFAULT '{}',
		set_at {{timestamp}} NOT NULL,
		met_at {{timestamp}},
		met_by TEXT,
		created_at {{timestamp}} NOT NULL,
		updated_at {{timestamp}} NOT NULL
	);
`

const phase2IndexesSQL = `
	CREATE INDEX IF NOT EXISTS idx_coordinator_watches_workflow ON coordinator_watches(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_watches_workspace ON coordinator_watches(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_activity_list ON coordinator_activity(coordinator_id, created_at DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_coordinator_activity_class ON coordinator_activity(coordinator_id, action_class, created_at);
	CREATE INDEX IF NOT EXISTS idx_coordinator_activity_created ON coordinator_activity(created_at);
	CREATE INDEX IF NOT EXISTS idx_coordinator_activity_workspace ON coordinator_activity(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_standing_orders_active ON coordinator_standing_orders(coordinator_id, retired_at, created_at, id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_standing_orders_workspace ON coordinator_standing_orders(workspace_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_goals_active ON coordinator_goals(coordinator_id) WHERE status = 'active';
	CREATE INDEX IF NOT EXISTS idx_coordinator_goals_workspace ON coordinator_goals(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_goals_history ON coordinator_goals(coordinator_id, created_at, id);
	CREATE UNIQUE INDEX IF NOT EXISTS coordinator_proposals_open_target ON coordinator_proposals(coordinator_id, kind, target_task_id)
		WHERE kind <> 'create_task' AND status IN ('pending','approving','failed');
`

// migratePhase2 applies the additive phase-2 schema: column migrations,
// then tables, then indexes over the new columns. Every step is replayable.
func (s *Store) migratePhase2(migrate *db.MigrateLogger) error {
	driver := s.db.DriverName()
	for _, m := range phase2ColumnMigrations {
		if err := migrate.Apply(m.name, dialect.MustRenderSchema(driver, m.stmt)); err != nil {
			return fmt.Errorf("coordinator phase 2 column %s: %w", m.name, err)
		}
	}
	if _, err := s.db.Exec(dialect.MustRenderSchema(driver, phase2TablesSQL)); err != nil {
		return fmt.Errorf("coordinator phase 2 tables: %w", err)
	}
	if _, err := s.db.Exec(dialect.MustRenderSchema(driver, phase2IndexesSQL)); err != nil {
		return fmt.Errorf("coordinator phase 2 indexes: %w", err)
	}
	return nil
}
