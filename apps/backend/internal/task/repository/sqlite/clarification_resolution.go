package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/task/models"
)

// Sentinel errors for clarification_resolutions access (spec M8, M8a, M9).
var (
	// ErrClarificationResolutionNotFound means no row exists for the given
	// pending_id — either it was never claimed, or (M9) a conflicting insert's
	// row disappeared (cascaded away) between the conflict and the read.
	ErrClarificationResolutionNotFound = errors.New("clarification resolution not found")
	// ErrClarificationSessionMissing means the claim insert failed the
	// session_id foreign key (M8a): either a claim-window race deleted the
	// session after step 1 read the bundle's messages, or the bundle is
	// orphaned (its session row is already gone).
	ErrClarificationSessionMissing = errors.New("clarification resolution: session missing")
)

const postgresForeignKeyViolation = "23503"

// sqliteForeignKeyViolationMessage is the substring go-sqlite3 puts in an FK
// violation error. clarification_resolutions declares exactly one foreign
// key (session_id), so a bare substring match attributes correctly.
const sqliteForeignKeyViolationMessage = "FOREIGN KEY constraint failed"

func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresForeignKeyViolation
	}
	return strings.Contains(err.Error(), sqliteForeignKeyViolationMessage)
}

// InsertClarificationResolution attempts to claim a clarification bundle
// (spec "The resolution claim", step 4 / M8). The insert has three possible
// outcomes:
//   - one row affected: this caller won the claim. Returns (true, res, nil).
//   - zero rows affected (pending_id conflict): someone else already won.
//     Returns (false, existingRow, nil). If the winning row has since
//     disappeared (M9), returns (false, nil, ErrClarificationResolutionNotFound).
//   - the insert fails its session_id foreign key (M8a): returns
//     (false, nil, ErrClarificationSessionMissing).
func (r *Repository) InsertClarificationResolution(ctx context.Context, res *models.ClarificationResolution) (bool, *models.ClarificationResolution, error) {
	query := r.db.Rebind(`
		INSERT INTO clarification_resolutions
			(pending_id, session_id, task_id, status, response, resume, resolved_by, source, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (pending_id) DO NOTHING
	`)
	result, err := r.db.ExecContext(ctx, query,
		res.PendingID, res.SessionID, res.TaskID, res.Status, res.Response, res.Resume, res.ResolvedBy, res.Source, res.ResolvedAt)
	if err != nil {
		if isForeignKeyViolation(err) {
			return false, nil, ErrClarificationSessionMissing
		}
		return false, nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, nil, err
	}
	if affected == 1 {
		return true, res, nil
	}

	existing, err := r.GetClarificationResolution(ctx, res.PendingID)
	if err != nil {
		return false, nil, err
	}
	return false, existing, nil
}

// GetClarificationResolution reads the resolution row for pending_id, or
// ErrClarificationResolutionNotFound if none exists.
func (r *Repository) GetClarificationResolution(ctx context.Context, pendingID string) (*models.ClarificationResolution, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT pending_id, session_id, task_id, status, response, resume, resolved_by, source, resolved_at
		FROM clarification_resolutions
		WHERE pending_id = ?
	`), pendingID)
	res, err := scanClarificationResolution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClarificationResolutionNotFound
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func scanClarificationResolution(row *sql.Row) (*models.ClarificationResolution, error) {
	var res models.ClarificationResolution
	err := row.Scan(
		&res.PendingID, &res.SessionID, &res.TaskID, &res.Status, &res.Response,
		&res.Resume, &res.ResolvedBy, &res.Source, &res.ResolvedAt,
	)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// UpdateClarificationResolutionResume records the resume outcome computed for
// an already-claimed bundle (spec M7). It is the only permitted post-claim
// mutation of a resolution row and touches no other column. Returns
// ErrClarificationResolutionNotFound if the row does not exist.
func (r *Repository) UpdateClarificationResolutionResume(ctx context.Context, pendingID, resume string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE clarification_resolutions SET resume = ? WHERE pending_id = ?
	`), resume, pendingID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrClarificationResolutionNotFound
	}
	return nil
}
