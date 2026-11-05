package sandbox

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

func insertRevisionContent(ctx context.Context, tx *sqlx.Tx, revision *Revision, raw []byte, expected int64) error {
	if revision.Migration != nil {
		if err := lockMigrationSource(ctx, tx, revision, expected); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO sandbox_policy_revisions (id,org_id,policy_id,revision,content_json) VALUES (?,?,?,?,?)`),
		revision.ID, revision.OrgID, revision.Policy.ID, revision.Number, string(raw))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

// Hold the source against revocation until the new revision commits or rolls back.
func lockMigrationSource(ctx context.Context, tx *sqlx.Tx, revision *Revision, expected int64) error {
	var sourceID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`UPDATE sandbox_policy_revisions SET revoked=revoked
 WHERE id=? AND org_id=? AND policy_id=? AND revision=? AND revoked=0 RETURNING id`),
		revision.Migration.SourceRevisionID, revision.OrgID, revision.Policy.ID, expected).Scan(&sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}
