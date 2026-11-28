package state

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
)

// ReportChangeRequestWithReceipt writes one change-request row and completes
// the command receipt in the same transaction. A replay therefore observes
// either both the row and the applied receipt or neither.
func (s *ChangeRequestStore) ReportChangeRequestWithReceipt(
	ctx context.Context,
	record ChangeRequestRecord,
	commands *CommandStore,
	operationID, resultStatus string,
) (ChangeRequestRecord, CommandRecord, error) {
	if commands == nil || commands.db != s.db {
		return ChangeRequestRecord{}, CommandRecord{}, errors.New("plugin task change request: command store is unavailable")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ChangeRequestRecord{}, CommandRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	stored, _, err := s.reportChangeRequestTx(ctx, tx, record)
	if err != nil {
		return ChangeRequestRecord{}, CommandRecord{}, err
	}
	completed, err := completeCommandInTx(ctx, tx, operationID, resultStatus, "", stored.ID, ChangeRequestResourceVersion(stored), "")
	if err != nil {
		return ChangeRequestRecord{}, CommandRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChangeRequestRecord{}, CommandRecord{}, err
	}
	return stored, completed, nil
}

// RemoveChangeRequestWithReceipt deletes one change-request row and completes
// the command receipt in the same transaction. Deleting a missing row is not an
// error. The identity carries no provider host: when it matches rows on several
// hosts nothing is deleted, ErrChangeRequestAmbiguous is returned, and the
// rollback leaves both the ledger and the receipt untouched.
func (s *ChangeRequestStore) RemoveChangeRequestWithReceipt(
	ctx context.Context,
	installationID, workspaceID, providerID, repositoryID string,
	number int64,
	commands *CommandStore,
	operationID, resultStatus string,
) (CommandRecord, error) {
	if commands == nil || commands.db != s.db {
		return CommandRecord{}, errors.New("plugin task change request: command store is unavailable")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return CommandRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := removeChangeRequestTx(ctx, tx, installationID, workspaceID, providerID, repositoryID, number); err != nil {
		return CommandRecord{}, err
	}
	completed, err := completeCommandInTx(ctx, tx, operationID, resultStatus, "", "", "", "")
	if err != nil {
		return CommandRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommandRecord{}, err
	}
	return completed, nil
}

// removeChangeRequestTx resolves the host-less public identity only when it
// names one stored object. A second host is an ambiguity, never permission to
// delete both objects or to guess which row the caller meant.
func removeChangeRequestTx(ctx context.Context, tx *sqlx.Tx, installationID, workspaceID, providerID, repositoryID string, number int64) error {
	if err := validateChangeRequestIdentity(installationID, workspaceID, providerID, repositoryID, number); err != nil {
		return err
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT id FROM plugin_task_change_requests
		WHERE installation_id = ? AND workspace_id = ? AND provider_id = ? AND repository_id = ? AND number = ?
		ORDER BY id LIMIT 2`), installationID, workspaceID, providerID, repositoryID, number)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	if len(ids) > 1 {
		return ErrChangeRequestAmbiguous
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM plugin_task_change_requests
		WHERE id = ? AND installation_id = ? AND workspace_id = ?`), ids[0], installationID, workspaceID)
	return err
}

func validateChangeRequestIdentity(installationID, workspaceID, providerID, repositoryID string, number int64) error {
	if installationID == "" || workspaceID == "" || providerID == "" || repositoryID == "" || number <= 0 {
		return errors.New("plugin task change request: required identity fields are missing")
	}
	return nil
}
