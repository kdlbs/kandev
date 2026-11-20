package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// BeginTurnChangeExportAttempt claims one bounded page attempt. A monotonic
// token makes an expired worker unable to overwrite its successor.
func (r *Repository) BeginTurnChangeExportAttempt(
	ctx context.Context,
	repositoryChangeID string,
	now time.Time,
	lease time.Duration,
	maxAttempts int64,
) (*models.TurnChangeExportClaim, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := lockTurnRepositoryChange(ctx, tx, r.db, repositoryChangeID)
	if err != nil {
		return nil, false, err
	}
	if !turnChangeExportAttemptEligible(row, now, maxAttempts) {
		return nil, false, nil
	}
	if err := validateTurnChangeContentParent(ctx, tx, r.db, row.TurnChangeSetID, now); err != nil {
		if errors.Is(err, errTurnChangeExportClaimLost) {
			return nil, false, nil
		}
		return nil, false, err
	}
	token := row.ExportClaimToken + 1
	leaseUntil := now.UTC().Add(lease)
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes
		   SET export_state = ?, export_attempts = export_attempts + 1,
		       export_claim_owner = ?, export_claim_token = ?, export_claim_expires_at = ?, updated_at = ?
		 WHERE id = ? AND export_claim_token = ?
	`), models.TurnChangeExportRunning, uuid.NewString(), token, leaseUntil, now.UTC(),
		repositoryChangeID, row.ExportClaimToken)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return turnChangeExportClaim(row, token, leaseUntil), true, nil
}

func turnChangeExportAttemptEligible(row *models.TurnRepositoryChangeSet, now time.Time, maxAttempts int64) bool {
	if row == nil || row.Availability != models.TurnChangeAvailabilityReady || row.ContentComplete {
		return false
	}
	if row.ExportAttempts >= maxAttempts {
		return false
	}
	switch row.ExportState {
	case "", models.TurnChangeExportQueued, models.TurnChangeExportRetrying, models.TurnChangeExportRunning:
	default:
		return false
	}
	if row.ExportState == models.TurnChangeExportRunning &&
		row.ExportClaimExpiresAt != nil && row.ExportClaimExpiresAt.After(now.UTC()) {
		return false
	}
	return row.ExportNextAttemptAt == nil || !row.ExportNextAttemptAt.After(now.UTC())
}

func turnChangeExportClaim(
	row *models.TurnRepositoryChangeSet,
	token int64,
	leaseUntil time.Time,
) *models.TurnChangeExportClaim {
	return &models.TurnChangeExportClaim{
		RepositoryChangeID: row.ID, ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID,
		Token: token, Attempt: row.ExportAttempts + 1, Cursor: row.ExportCursor,
		ExportBytes: row.ExportBytes, Fingerprint: row.ExportFingerprint,
		ComparisonVersion: row.ComparisonVersion, ComparisonBasis: row.ComparisonBasis,
		StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
		EffectiveStartCommitOID: row.EffectiveStartCommitOID, EffectiveStartTreeOID: row.EffectiveStartTreeOID,
		EffectiveEndCommitOID: row.EffectiveEndCommitOID, EffectiveEndTreeOID: row.EffectiveEndTreeOID,
		ComparisonRef: row.ComparisonRef, LeaseUntil: leaseUntil,
	}
}

// StoreTurnChangeExportPage commits page content, compressed-byte accounting,
// the next cursor, and the resulting export state in one transaction.
func (r *Repository) StoreTurnChangeExportPage(
	ctx context.Context,
	claim models.TurnChangeExportClaim,
	files []models.TurnChangeFileContent,
	page models.TurnChangeExportPage,
) (models.TurnChangeContentStoreReceipt, bool, error) {
	prepared, _, err := prepareTurnChangeFileContents(files)
	if err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	changeSetID, checkoutID, remaining, err := prepareTurnChangeContentWrite(ctx, tx, r.db, claim.RepositoryChangeID)
	if err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	if err := validateTurnChangeContentParent(ctx, tx, r.db, changeSetID, time.Now().UTC()); err != nil {
		if errors.Is(err, errTurnChangeExportClaimLost) {
			return models.TurnChangeContentStoreReceipt{}, false, nil
		}
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	if err := validateCurrentTurnChangeExportClaim(ctx, tx, r.db, claim); err != nil {
		if errors.Is(err, errTurnChangeExportClaimLost) {
			return models.TurnChangeContentStoreReceipt{}, false, nil
		}
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	storedBytes, err := storeTurnChangeExportPageFiles(ctx, tx, r.db, claim.RepositoryChangeID, checkoutID, prepared, &remaining)
	if err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	page.BatchBytes = storedBytes
	receipt, err := settleTurnChangeContentReceipt(ctx, tx, r.db, claim.RepositoryChangeID, changeSetID, storedBytes, time.Now().UTC())
	if err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	if err := updateTurnChangeExportProgress(ctx, tx, r.db, claim, page, receipt); err != nil {
		if errors.Is(err, errTurnChangeExportClaimLost) {
			return models.TurnChangeContentStoreReceipt{}, false, nil
		}
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return models.TurnChangeContentStoreReceipt{}, false, err
	}
	return receipt, true, nil
}

func storeTurnChangeExportPageFiles(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	repositoryChangeID, checkoutID string,
	prepared []models.TurnChangeFileContent,
	remaining *int64,
) (int64, error) {
	now := time.Now().UTC()
	storedBytes := int64(0)
	for index := range prepared {
		if err := prepareTurnChangeFileRecord(ctx, tx, db, repositoryChangeID, checkoutID, &prepared[index].File, now); err != nil {
			return 0, err
		}
		partial, bytes, err := storeTurnChangeFilePayloads(ctx, tx, db, &prepared[index], remaining, now)
		if err != nil {
			return 0, err
		}
		storedBytes += bytes
		if err := finishStoredTurnChangeFile(ctx, tx, db, &prepared[index], partial); err != nil {
			return 0, err
		}
	}
	return storedBytes, nil
}

func validateCurrentTurnChangeExportClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	claim models.TurnChangeExportClaim,
) error {
	query := `SELECT export_state FROM turn_repository_changes
		WHERE id = ? AND export_claim_token = ? AND export_state = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var state string
	err := tx.GetContext(ctx, &state, db.Rebind(query), claim.RepositoryChangeID, claim.Token, models.TurnChangeExportRunning)
	if errors.Is(err, sql.ErrNoRows) {
		return errTurnChangeExportClaimLost
	}
	return err
}

func validateTurnChangeContentParent(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID string,
	now time.Time,
) error {
	var availability models.TurnChangeAvailability
	var retainUntil sql.NullTime
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT availability, retain_until FROM turn_change_sets WHERE id = ?
	`), changeSetID).Scan(&availability, &retainUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return errTurnChangeExportClaimLost
	}
	if err != nil {
		return err
	}
	if availability != models.TurnChangeAvailabilityReady {
		return errTurnChangeExportClaimLost
	}
	if retainUntil.Valid && !retainUntil.Time.After(now.UTC()) {
		return errTurnChangeExportClaimLost
	}
	return nil
}

var errTurnChangeExportClaimLost = errors.New("turn change export claim was taken over")

func updateTurnChangeExportProgress(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	claim models.TurnChangeExportClaim,
	page models.TurnChangeExportPage,
	receipt models.TurnChangeContentStoreReceipt,
) error {
	state, reason := turnChangeExportPageState(page, receipt)
	complete := page.Complete && receipt.Complete && !page.Failed && !page.Retry
	keepClaim := page.KeepClaim && !page.Complete && !page.Failed && !page.Retry
	query := `
		UPDATE turn_repository_changes
		   SET export_state = ?, export_cursor = ?, export_bytes = export_bytes + ?,
		       export_reason = ?, export_fingerprint = ?, export_next_attempt_at = ?,
		       content_complete = ?,
		       export_claim_owner = CASE WHEN ? THEN export_claim_owner ELSE '' END,
		       export_claim_expires_at = CASE WHEN ? THEN export_claim_expires_at ELSE NULL END,
		       updated_at = ?
		 WHERE id = ? AND export_claim_token = ? AND export_state = ?
	`
	result, err := tx.ExecContext(ctx, db.Rebind(query), state, page.NextCursor, page.BatchBytes,
		reason, page.Fingerprint, page.NextAttemptAt, complete, keepClaim, keepClaim,
		time.Now().UTC(), claim.RepositoryChangeID, claim.Token, models.TurnChangeExportRunning)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errTurnChangeExportClaimLost
	}
	return nil
}

func turnChangeExportPageState(page models.TurnChangeExportPage, receipt models.TurnChangeContentStoreReceipt) (models.TurnChangeExportState, models.TurnChangeReason) {
	if page.Failed {
		return models.TurnChangeExportUnavailable, pageFailureReason(page, receipt)
	}
	if page.Complete && receipt.Complete {
		return models.TurnChangeExportComplete, ""
	}
	if page.Complete && !receipt.Pending {
		return models.TurnChangeExportUnavailable, pageFailureReason(page, receipt)
	}
	if page.Retry {
		return models.TurnChangeExportRetrying, page.Reason
	}
	if page.Complete && receipt.Pending {
		return models.TurnChangeExportRetrying, models.TurnChangeReasonContentUnavailable
	}
	if page.KeepClaim {
		return models.TurnChangeExportRunning, ""
	}
	return models.TurnChangeExportQueued, ""
}

func pageFailureReason(page models.TurnChangeExportPage, receipt models.TurnChangeContentStoreReceipt) models.TurnChangeReason {
	if page.Reason != "" {
		return page.Reason
	}
	if receipt.Reason != "" {
		return receipt.Reason
	}
	return models.TurnChangeReasonExportInterrupted
}

// ReleaseTurnChangeExportClaim returns an interrupted page to the queue without
// consuming another attempt.
func (r *Repository) ReleaseTurnChangeExportClaim(ctx context.Context, claim models.TurnChangeExportClaim) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET export_state = ?, export_claim_owner = '',
			export_claim_expires_at = NULL, export_next_attempt_at = ?, updated_at = ?
		WHERE id = ? AND export_claim_token = ? AND export_state = ?
	`), models.TurnChangeExportRetrying, time.Now().UTC(), time.Now().UTC(),
		claim.RepositoryChangeID, claim.Token, models.TurnChangeExportRunning)
	return err
}

// ListTurnChangeExportCandidates returns rows that a bounded processor may
// claim now. Expired claims are eligible so a crashed worker cannot strand a
// row forever.
func (r *Repository) ListTurnChangeExportCandidates(ctx context.Context, now time.Time, limit int) ([]*models.TurnRepositoryChangeSet, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT `+turnRepositoryChangeColumns+`
		FROM turn_repository_changes rc
		WHERE rc.availability = ? AND rc.content_complete = ?
		  AND rc.preservation_state = ''
		  AND EXISTS (
		    SELECT 1 FROM turn_change_sets cs
		    WHERE cs.id = rc.change_set_id AND cs.availability = ?
		      AND (cs.retain_until IS NULL OR cs.retain_until > ?)
		  )
		  AND rc.export_state IN (?, ?, ?, ?)
		  AND (rc.export_next_attempt_at IS NULL OR rc.export_next_attempt_at <= ?)
		  AND (rc.export_claim_expires_at IS NULL OR rc.export_claim_expires_at <= ?)
		ORDER BY rc.updated_at, rc.id
		LIMIT ?
	`), models.TurnChangeAvailabilityReady, false, models.TurnChangeAvailabilityReady, now.UTC(),
		"", models.TurnChangeExportQueued, models.TurnChangeExportRetrying, models.TurnChangeExportRunning,
		now.UTC(), now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := decodeTurnChangeOverlaps(row.OverlapIntervalsJSON, &row.OverlapIntervals); err != nil {
			return nil, fmt.Errorf("decode repository change overlaps: %w", err)
		}
	}
	return rows, nil
}

func lockTurnRepositoryChange(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	id string,
) (*models.TurnRepositoryChangeSet, error) {
	query := `SELECT ` + turnRepositoryChangeColumns + ` FROM turn_repository_changes WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	row := &models.TurnRepositoryChangeSet{}
	if err := tx.GetContext(ctx, row, db.Rebind(query), id); err != nil {
		return nil, err
	}
	return row, nil
}

// SettleTurnChangeContentProgress recomputes content completeness from every
// stored file, not from the page that just arrived, and publishes a revision.
func (r *Repository) SettleTurnChangeContentProgress(ctx context.Context, changeSetID string) error {
	if changeSetID == "" {
		return fmt.Errorf("settle turn change content: change set identity is required")
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets
		   SET content_complete = (
		         NOT EXISTS (
		           SELECT 1 FROM turn_repository_changes
		           WHERE change_set_id = ? AND availability = ? AND content_complete = ?
		         )
		         AND NOT EXISTS (
		           SELECT 1 FROM turn_repository_changes
		           WHERE change_set_id = ? AND availability <> ?
		         )
		       ),
		       complete = summary_complete AND (
		         NOT EXISTS (
		           SELECT 1 FROM turn_repository_changes
		           WHERE change_set_id = ? AND availability = ? AND content_complete = ?
		         )
		         AND NOT EXISTS (
		           SELECT 1 FROM turn_repository_changes
		           WHERE change_set_id = ? AND availability <> ?
		         )
		       ),
		       revision = revision + 1, updated_at = ?
		 WHERE id = ? AND terminal_at IS NOT NULL AND availability = ?
	`), changeSetID, models.TurnChangeAvailabilityReady, false,
		changeSetID, models.TurnChangeAvailabilityReady,
		changeSetID, models.TurnChangeAvailabilityReady, false,
		changeSetID, models.TurnChangeAvailabilityReady,
		now, changeSetID, models.TurnChangeAvailabilityReady)
	if err != nil {
		return err
	}
	if _, err := result.RowsAffected(); err != nil {
		return err
	}
	return nil
}

// SettleTurnRepositoryExportFailure records a typed, terminal content
// failure only while the caller still owns the active claim token.
func (r *Repository) SettleTurnRepositoryExportFailure(
	ctx context.Context,
	repositoryChangeID string,
	claimToken int64,
	reason models.TurnChangeReason,
) error {
	if repositoryChangeID == "" || !reason.Valid() {
		return fmt.Errorf("settle turn repository export: invalid identity or reason")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes
		   SET export_state = ?, export_reason = ?, content_complete = ?,
		       export_claim_owner = '', export_claim_expires_at = NULL, updated_at = ?
		 WHERE id = ? AND export_claim_token = ? AND export_state <> ?
	`), models.TurnChangeExportUnavailable, reason, false, time.Now().UTC(),
		repositoryChangeID, claimToken, models.TurnChangeExportComplete)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_file_changes
		   SET content_availability = ?, content_reason = ?, content_truncated = ?
		 WHERE repository_change_id = ? AND content_availability = ?
	`), models.TurnChangeAvailabilityUnavailable, reason, true, repositoryChangeID, models.TurnChangeAvailabilityPending); err != nil {
		return err
	}
	return tx.Commit()
}
