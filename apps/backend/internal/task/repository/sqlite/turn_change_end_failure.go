package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// SetTurnRepositoryEndUnavailable settles only the failed checkout. The stored
// result prevents a retry from substituting a later end snapshot.
func (r *Repository) SetTurnRepositoryEndUnavailable(ctx context.Context, changeSetID, repositoryChangeID, startCommitOID, startTreeOID string, reason models.TurnChangeReason) (bool, error) {
	if !validUnavailableTurnRepositoryEnd(changeSetID, repositoryChangeID, startCommitOID, startTreeOID, reason) {
		return false, fmt.Errorf("settle unavailable turn repository end: endpoint identity and reason are required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	set, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID)
	if err != nil {
		return false, turnChangeSetLockError(err)
	}
	var row models.TurnRepositoryChangeSet
	err = tx.GetContext(ctx, &row, r.db.Rebind(`SELECT id, availability, end_commit_oid FROM turn_repository_changes WHERE id = ? AND change_set_id = ? AND start_commit_oid = ? AND start_tree_oid = ?`), repositoryChangeID, changeSetID, startCommitOID, startTreeOID)
	if err != nil {
		return false, err
	}
	if row.EndCommitOID != "" || row.Availability == models.TurnChangeAvailabilityUnavailable {
		return true, nil
	}
	if set.TerminalAt != nil || row.Availability != models.TurnChangeAvailabilityPending {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE turn_repository_changes SET availability = 'unavailable', reason = ?, updated_at = ? WHERE id = ? AND change_set_id = ?`), reason, time.Now().UTC(), repositoryChangeID, changeSetID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validUnavailableTurnRepositoryEnd(changeSetID, repositoryChangeID, startCommitOID, startTreeOID string, reason models.TurnChangeReason) bool {
	return changeSetID != "" && repositoryChangeID != "" && startCommitOID != "" && startTreeOID != "" && reason != "" && reason.Valid()
}
