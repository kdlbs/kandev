package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db/dialect"
)

// SidebarTaskOrderingProfile identifies executable local/server ordering parity.
func (r *Repository) SidebarTaskOrderingProfile() string {
	if dialect.IsPostgres(r.ro.DriverName()) {
		return "server_only"
	}
	return "sqlite_nocase_v1"
}

// SidebarTaskWorkflowIDs returns scope identities without hydrating task records.
func (r *Repository) SidebarTaskWorkflowIDs(ctx context.Context, workspaceID string) ([]string, error) {
	ids := make([]string, 0)
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`
		SELECT DISTINCT COALESCE(workflow_id, '') FROM tasks t
		WHERE t.workspace_id = ? AND t.archived_at IS NULL
			AND COALESCE(t.is_ephemeral, 0) = 0
			AND COALESCE(t.origin, '') <> 'automation_run'
			AND `+excludeConfigModePredicate(r.ro.DriverName(), "t.metadata")), workspaceID)
	return ids, err
}
