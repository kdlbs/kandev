package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// usageLiveTranscriptExpression is also used by the schema owner for the
// matching expression index. Invalid legacy metadata provides no source proof.
func usageLiveTranscriptExpression(driver, column string) string {
	if driver == usageDriverPostgres {
		return `CASE WHEN ` + column + ` IS JSON OBJECT THEN ` + column + `::json->'acp'->>'session_id' END`
	}
	return `CASE WHEN json_valid(` + column + `) THEN json_extract(` + column + `, '$.acp.session_id') END`
}

func (r *Repository) usageLiveSourceCrossesScopeTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget) (bool, error) {
	if target.ExternalSessionID == "" {
		return false, nil
	}
	var organization string
	if err := tx.GetContext(ctx, &organization, r.db.Rebind(`SELECT organization_id FROM usage_scopes WHERE scope_id = ?`), target.ScopeID); err != nil {
		return false, err
	}
	var foreign bool
	query := `SELECT EXISTS(SELECT 1 FROM task_sessions ts JOIN tasks t ON t.id=ts.task_id JOIN workspaces w ON w.id=t.workspace_id
		WHERE ` + usageLiveTranscriptExpression(r.db.DriverName(), "ts.metadata") + ` = ? AND (w.id <> ? OR COALESCE(w.org_id,'') <> ?))`
	if err := tx.GetContext(ctx, &foreign, r.db.Rebind(query), target.ExternalSessionID, target.WorkspaceID, organization); err != nil {
		return false, err
	}
	if foreign {
		return true, nil
	}
	return r.usageRunningSourceCrossesScopeTx(ctx, tx, target, organization)
}

func (r *Repository) usageRunningSourceCrossesScopeTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, organization string) (bool, error) {
	var foreign bool
	err := tx.GetContext(ctx, &foreign, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM executors_running e
		JOIN task_sessions ts ON ts.id=e.session_id JOIN tasks t ON t.id=ts.task_id JOIN workspaces w ON w.id=t.workspace_id
		WHERE e.resume_token = ? AND (w.id <> ? OR COALESCE(w.org_id,'') <> ?))`), target.ExternalSessionID, target.WorkspaceID, organization)
	return foreign, err
}
