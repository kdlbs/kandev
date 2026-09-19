package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

type createTurnTxOptions struct {
	stampStep bool
	receipt   bool
}

type turnSessionAuthority struct {
	taskID             string
	queueIncarnationID string
	environmentID      sql.NullString
}

func (r *Repository) createTurnTx(
	ctx context.Context,
	turn *models.Turn,
	options createTurnTxOptions,
) (bool, *models.ConversationMutationReceipt, error) {
	stampTurnDefaults(turn)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, nil, fmt.Errorf("begin turn creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	authorityTaskID, err := r.admitSessionWriteTx(ctx, tx, turn.TaskSessionID)
	if err != nil {
		return false, nil, err
	}

	stamped, err := r.stampTurnStepTx(ctx, tx, turn, authorityTaskID, options.stampStep)
	if err != nil {
		return false, nil, err
	}
	base, err := r.prepareTurnReceiptTx(ctx, tx, turn.TaskSessionID, options.receipt)
	if err != nil {
		return false, nil, err
	}
	if err := r.insertTurnRow(ctx, tx, turn); err != nil {
		return false, nil, err
	}
	receipt, err := r.createdTurnReceiptTx(ctx, tx, turn, base, options.receipt)
	if err != nil {
		return false, nil, err
	}
	if err := tx.Commit(); err != nil {
		return false, nil, fmt.Errorf("commit turn creation: %w", err)
	}
	return stamped, receipt, nil
}

// admitSessionWriteTx orders every turn write behind task and environment
// recovery authority before taking the session-scoped turn lock.
func (r *Repository) admitSessionWriteTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (string, error) {
	authority, err := r.readTurnSessionAuthorityTx(ctx, tx, sessionID)
	if err != nil {
		return "", err
	}
	lockTaskID, err := r.turnSessionAuthorityTaskTx(ctx, tx, authority)
	if err != nil {
		return "", err
	}
	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), lockTaskID); err != nil {
		return "", fmt.Errorf("lock task for turn creation: %w", err)
	}
	confirmed, err := r.confirmTurnSessionAuthorityTx(ctx, tx, sessionID, authority, lockTaskID)
	if err != nil {
		return "", err
	}
	if err := r.ensureTurnSessionRecoveryAvailableTx(ctx, tx, sessionID, confirmed); err != nil {
		return "", err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return "", err
	}
	if _, err := r.confirmTurnSessionAuthorityTx(ctx, tx, sessionID, confirmed, lockTaskID); err != nil {
		return "", err
	}
	return lockTaskID, nil
}

func (r *Repository) confirmTurnSessionAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
	expected turnSessionAuthority,
	expectedTaskID string,
) (turnSessionAuthority, error) {
	current, err := r.readTurnSessionAuthorityTx(ctx, tx, sessionID)
	if err != nil {
		return turnSessionAuthority{}, err
	}
	currentTaskID, err := r.turnSessionAuthorityTaskTx(ctx, tx, current)
	if err != nil {
		return turnSessionAuthority{}, err
	}
	if !sameTurnSessionAuthority(current, expected) || currentTaskID != expectedTaskID {
		return turnSessionAuthority{}, fmt.Errorf("turn session %s authority changed", sessionID)
	}
	return current, nil
}

func (r *Repository) ensureTurnSessionRecoveryAvailableTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
	authority turnSessionAuthority,
) error {
	if !authority.environmentID.Valid || authority.environmentID.String == "" {
		return nil
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, authority.environmentID.String); err != nil {
		return err
	}
	if claim := recoveryclaim.ClaimFromContext(ctx); claim != nil &&
		claim.TaskEnvironmentID == authority.environmentID.String && claim.SessionID != sessionID {
		return fmt.Errorf("%w: environment %s is claimed by session %s",
			recoveryclaim.ErrBusy, authority.environmentID.String, claim.SessionID)
	}
	return nil
}

func sameTurnSessionAuthority(left, right turnSessionAuthority) bool {
	return left.taskID == right.taskID &&
		left.queueIncarnationID == right.queueIncarnationID &&
		left.environmentID == right.environmentID
}

func (r *Repository) readTurnSessionAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (turnSessionAuthority, error) {
	authority := turnSessionAuthority{}
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, queue_incarnation_id, task_environment_id FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&authority.taskID, &authority.queueIncarnationID, &authority.environmentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authority, fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, sessionID)
		}
		return authority, err
	}
	return authority, nil
}

func (r *Repository) turnSessionAuthorityTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	authority turnSessionAuthority,
) (string, error) {
	if !authority.environmentID.Valid || authority.environmentID.String == "" {
		return authority.taskID, nil
	}
	var ownerTaskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id FROM task_environments WHERE id = ?
	`), authority.environmentID.String).Scan(&ownerTaskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authority.taskID, nil
		}
		return "", err
	}
	return ownerTaskID, nil
}

func (r *Repository) stampTurnStepTx(
	ctx context.Context,
	tx *sqlx.Tx,
	turn *models.Turn,
	authorityTaskID string,
	enabled bool,
) (bool, error) {
	if !enabled || turn.TaskID != authorityTaskID {
		return false, nil
	}
	_, stepID, found, err := r.readTaskStepInTx(ctx, tx, turn.TaskID)
	if err != nil {
		return false, err
	}
	if !found || stepID == "" {
		return false, nil
	}
	if turn.Metadata == nil {
		turn.Metadata = map[string]interface{}{}
	}
	turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] = stepID
	return true, nil
}

func (r *Repository) prepareTurnReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
	enabled bool,
) (int64, error) {
	if !enabled {
		return 0, nil
	}
	return r.ensureConversationRevisionTx(ctx, tx, sessionID)
}

func (r *Repository) createdTurnReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	turn *models.Turn,
	base int64,
	enabled bool,
) (*models.ConversationMutationReceipt, error) {
	if !enabled {
		return nil, nil
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationTurnReceipt(
		ctx, tx, receipt, base, turn, models.ConversationMutationUpsert,
	); err != nil {
		return nil, err
	}
	return receipt, nil
}
