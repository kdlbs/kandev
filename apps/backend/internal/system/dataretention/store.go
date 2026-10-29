package dataretention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
)

const settingsKey = "archived_data_retention"

func readRecord(ctx context.Context, q sqlx.QueryerContext) (record, error) {
	var raw string
	err := sqlx.GetContext(ctx, q, &raw, `SELECT value FROM settings WHERE key=?`, settingsKey)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultRecord(), nil
	}
	if err != nil {
		return record{}, err
	}
	var r record
	if json.Unmarshal([]byte(raw), &r) != nil || r.Version != 1 || r.Policy.Revision < 0 {
		return record{}, errors.New("invalid_stored_policy")
	}
	if r.Policy.ArchivedAge.Validate() != nil || r.Policy.CleanupAge.Validate() != nil {
		return record{}, errors.New("invalid_stored_policy")
	}
	if err := validateRecord(r); err != nil {
		return record{}, err
	}
	r.Supported = true
	return r, nil
}

func validateRecord(r record) error {
	if err := validatePreparation(r); err != nil {
		return err
	}
	return validateOperation(r.Operation)
}

func validatePreparation(r record) error {
	switch r.Preparation.State {
	case stateNone, statePending, stateRunning, stateFailed, stateReady:
	default:
		return errors.New("invalid_preparation")
	}
	if r.Preparation.State != stateNone && r.Preparation.Choice != choiceBackup && r.Preparation.Choice != choiceSkip {
		return errors.New("invalid_preparation")
	}
	if r.Policy.Enabled && r.ApprovedRevision != r.Policy.Revision {
		return errors.New("invalid_preparation")
	}
	return nil
}

func validateOperation(op *Operation) error {
	if op == nil {
		return nil
	}
	if op.ID == "" {
		return errors.New("invalid_operation")
	}
	switch op.Kind {
	case kindAnalysis, kindCleanup, choiceBackup:
	default:
		return errors.New("invalid_operation")
	}
	switch op.State {
	case stateRunning, stateSucceeded, stateFailed, statePartial, stateCancelled:
	default:
		return errors.New("invalid_operation")
	}
	return nil
}

func writeRecord(ctx context.Context, tx *sqlx.Tx, r record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
	 ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`), settingsKey, string(b))
	return err
}

func (s *Service) change(ctx context.Context, fn func(*record, *sqlx.Tx) error) (record, error) {
	if !s.supported() {
		return record{}, errors.New("unsupported")
	}
	tx, err := s.pool.Writer().BeginTxx(ctx, nil)
	if err != nil {
		return record{}, err
	}
	defer tx.Rollback() //nolint:errcheck // A committed transaction is already closed.
	// Acquire the writer lock before reading either policy or task admission, so
	// task admission, message writes and cleanup serialize through it.
	if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE settings SET key=key WHERE key=?`), settingsKey); err != nil {
		return record{}, err
	}
	r, err := readRecord(ctx, tx)
	if err != nil {
		return r, err
	}
	if err = fn(&r, tx); err != nil {
		return r, err
	}
	if err = writeRecord(ctx, tx, r); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
