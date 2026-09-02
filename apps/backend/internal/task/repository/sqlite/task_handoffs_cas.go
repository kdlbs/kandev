package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// GetTaskHandoffsRaw returns the current raw JSON value of the task's
// handoffs metadata array ("" if absent), for use as the expected value in a
// subsequent SetTaskHandoffsIfUnchanged call.
func (r *Repository) GetTaskHandoffsRaw(ctx context.Context, taskID string) (string, error) {
	var raw sql.NullString
	query := r.db.Rebind("SELECT metadata FROM tasks WHERE id = ?")
	if err := r.db.QueryRowxContext(ctx, query, taskID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return "", repoerrors.ErrTaskNotFound
		}
		return "", err
	}
	if !raw.Valid {
		return "", nil
	}
	return handoffsRawValue(raw.String)
}

// SetTaskHandoffsIfUnchanged stores newHandoffsJSON as the task's handoffs
// metadata value only if the current raw JSON for that key still equals
// expectedHandoffsJSON. On a mismatch it returns the current raw JSON (and
// stored=false, err=nil) so the caller can recompute its append against the
// latest value and retry, per the tool's bounded optimistic-CAS retry loop.
func (r *Repository) SetTaskHandoffsIfUnchanged(
	ctx context.Context,
	taskID, expectedHandoffsJSON, newHandoffsJSON string,
) (stored bool, currentHandoffsJSON string, err error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = tx.Rollback() }()

	metadata, err := r.lockTaskRowForHandoffs(ctx, tx, taskID)
	if err != nil {
		return false, "", err
	}
	current, err := handoffsRawValue(metadata)
	if err != nil {
		return false, "", err
	}
	if current != expectedHandoffsJSON {
		if err := tx.Commit(); err != nil {
			return false, "", err
		}
		return false, current, nil
	}

	query := metadataKeyUpdateQuery("tasks", r.db.DriverName())
	args := metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyHandoffs, newHandoffsJSON, r.nowUTC(), taskID)
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return false, "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, "", err
	}
	if rows == 0 {
		return false, "", repoerrors.ErrTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, "", err
	}
	return true, newHandoffsJSON, nil
}

// forUpdateSuffix locks the row for the duration of the transaction on
// Postgres; SQLite has no row-level locking and relies on its own
// transaction serialization instead (matching lockMetadataRow's precedent).
const forUpdateSuffix = " FOR UPDATE"

func (r *Repository) lockTaskRowForHandoffs(ctx context.Context, tx *sqlx.Tx, taskID string) (string, error) {
	query := "SELECT metadata FROM tasks WHERE id = ?"
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateSuffix
	}
	var raw sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), taskID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return "", repoerrors.ErrTaskNotFound
		}
		return "", err
	}
	if !raw.Valid {
		return "{}", nil
	}
	return raw.String, nil
}

// handoffsRawValue extracts the raw JSON bytes stored under MetaKeyHandoffs
// from a task's metadata JSON blob, returning "" only when the key is
// genuinely absent (including when the whole metadata blob is absent or
// null). Per AC-27, an absent key compares equal to the empty array, but a
// key that is *present* with an explicit null value is a distinct, corrupt
// shape ("present but not an array") — so a present null is returned as the
// literal string "null" rather than being collapsed into the same "" that
// means "absent", letting the caller (parseHandoffEntries) tell the two
// apart.
func handoffsRawValue(metadataJSON string) (string, error) {
	if strings.TrimSpace(metadataJSON) == "" || strings.TrimSpace(metadataJSON) == jsonNull {
		return "", nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return "", fmt.Errorf("failed to parse metadata: %w", err)
	}
	raw, ok := metadata[models.MetaKeyHandoffs]
	if !ok {
		return "", nil
	}
	return string(raw), nil
}
