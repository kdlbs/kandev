package state

import (
	"context"
	"errors"
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
	if commands == nil {
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
// error.
func (s *ChangeRequestStore) RemoveChangeRequestWithReceipt(
	ctx context.Context,
	installationID, workspaceID, providerID, repositoryID string,
	number int64,
	commands *CommandStore,
	operationID, resultStatus string,
) (CommandRecord, error) {
	if commands == nil {
		return CommandRecord{}, errors.New("plugin task change request: command store is unavailable")
	}
	if err := validateChangeRequestIdentity(installationID, workspaceID, providerID, repositoryID, number); err != nil {
		return CommandRecord{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return CommandRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM plugin_task_change_requests
		WHERE installation_id = ? AND workspace_id = ? AND provider_id = ? AND repository_id = ? AND number = ?
	`), installationID, workspaceID, providerID, repositoryID, number); err != nil {
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

func validateChangeRequestIdentity(installationID, workspaceID, providerID, repositoryID string, number int64) error {
	if installationID == "" || workspaceID == "" || providerID == "" || repositoryID == "" || number <= 0 {
		return errors.New("plugin task change request: required identity fields are missing")
	}
	return nil
}
