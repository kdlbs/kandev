package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

type currentUsageSessionKey struct {
	TaskID    string `db:"task_id"`
	SessionID string `db:"session_id"`
}

func (r *Repository) currentTranscriptLiveSessionsTx(ctx context.Context, tx *sqlx.Tx, workspace string, query currentUsageTargetSelector) ([]currentUsageSessionKey, error) {
	metadata, err := db.ColumnExistsContext(ctx, tx, "task_sessions", "metadata")
	if err != nil {
		return nil, err
	}
	var rows []currentUsageSessionKey
	if metadata {
		statement := `SELECT ts.task_id,ts.id AS session_id FROM task_sessions ts JOIN tasks t ON t.id=ts.task_id WHERE
			` + usageLiveTranscriptExpression(r.db.DriverName(), "ts.metadata") + ` = ? AND t.workspace_id = ?`
		if err := r.selectCurrentTranscriptSessionsTx(ctx, tx, statement, workspace, query, &rows); err != nil {
			return nil, err
		}
		if len(rows) > 1 {
			return rows, nil
		}
	}
	has, err := db.TableExistsContext(ctx, tx, "executors_running")
	if err != nil || !has {
		return rows, err
	}
	statement := `SELECT ts.task_id,ts.id AS session_id FROM executors_running e JOIN task_sessions ts ON ts.id=e.session_id
		JOIN tasks t ON t.id=ts.task_id WHERE e.resume_token = ? AND t.workspace_id = ?`
	if metadata {
		statement += ` AND COALESCE(` + usageLiveTranscriptExpression(r.db.DriverName(), "ts.metadata") + `,'') = ''`
	}
	var running []currentUsageSessionKey
	if err := r.selectCurrentTranscriptSessionsTx(ctx, tx, statement, workspace, query, &running); err != nil {
		return nil, err
	}
	for _, row := range running {
		if len(rows) == 0 || rows[0] != row {
			rows = append(rows, row)
		}
		if len(rows) > 1 {
			break
		}
	}
	return rows, nil
}

func (r *Repository) selectCurrentTranscriptSessionsTx(ctx context.Context, tx *sqlx.Tx, statement, workspace string, query currentUsageTargetSelector, rows *[]currentUsageSessionKey) error {
	args := []any{query.ExternalSessionID, workspace}
	if query.TaskID != "" {
		statement += ` AND ts.task_id = ?`
		args = append(args, query.TaskID)
	}
	return tx.SelectContext(ctx, rows, r.db.Rebind(statement+` LIMIT 2`), args...)
}
