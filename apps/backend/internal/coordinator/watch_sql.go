package coordinator

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/coordinator/watch"
)

// ResolveWatchProjectsSQL gives recording readers the same live project
// resolution as task-service readers, without invoking repairing store reads.
func ResolveWatchProjectsSQL(ctx context.Context, db *sqlx.DB, workspaceID string, scope watch.Scope) (watch.Resolved, error) {
	return watch.NewResolver(watchSQLReader{db: db}).Resolve(ctx, workspaceID, scope)
}

type watchSQLReader struct{ db *sqlx.DB }

func (r watchSQLReader) Sets(ctx context.Context, workspaceID string) ([]watch.SetMembers, error) {
	var rows []struct {
		ID           string `db:"id"`
		RepositoryID string `db:"repository_id"`
	}
	err := r.db.SelectContext(ctx, &rows, r.db.Rebind(`SELECT rs.id, COALESCE(repo.id, '') AS repository_id
		FROM repository_sets rs LEFT JOIN repository_set_items ri ON ri.repository_set_id = rs.id
		LEFT JOIN repositories repo ON repo.id = ri.repository_id AND repo.workspace_id = rs.workspace_id AND repo.deleted_at IS NULL
		WHERE rs.workspace_id = ? ORDER BY rs.id, repo.id`), workspaceID)
	if err != nil {
		return nil, err
	}
	var sets []watch.SetMembers
	for _, row := range rows {
		if len(sets) == 0 || sets[len(sets)-1].ID != row.ID {
			sets = append(sets, watch.SetMembers{ID: row.ID})
		}
		if row.RepositoryID != "" {
			last := &sets[len(sets)-1]
			last.RepositoryIDs = append(last.RepositoryIDs, row.RepositoryID)
		}
	}
	return sets, nil
}

func (r watchSQLReader) RepositoryIDs(ctx context.Context, workspaceID string) ([]string, error) {
	var ids []string
	err := r.db.SelectContext(ctx, &ids, r.db.Rebind(`SELECT id FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY id`), workspaceID)
	return ids, err
}
