package toolretention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

const settingsKey = "tool_payload_retention"

// recordVersion 2 is the combined tool-detail and historical-content scope.
// Version 1 tool-only approvals must not authorize the wider destructive scope.
const recordVersion = 2

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
	if json.Unmarshal([]byte(raw), &r) != nil || (r.Version != 1 && r.Version != recordVersion) ||
		r.Policy.Age.Validate() != nil || r.Policy.Revision < 0 {
		return record{}, errors.New("invalid_stored_policy")
	}
	legacyApproval := r.Version == 1
	if legacyApproval && validateRecord(r) != nil {
		return record{}, errors.New("invalid_stored_policy")
	}
	switch r.Preparation.State {
	case stateNone, statePending, stateRunning, stateFailed, stateReady:
	default:
		return record{}, errors.New("invalid_preparation")
	}
	if err := validateRecord(r); err != nil {
		return record{}, err
	}
	if legacyApproval {
		r = migrateToolOnlyRecord(r)
	}
	r.Supported = true
	if r.Operation != nil && r.Operation.Skipped == nil {
		r.Operation.Skipped = map[string]int64{}
	}
	return r, nil
}

// migrateToolOnlyRecord preserves the selected period and revision but clears
// the old approval. The wider scope requires the existing preparation choice
// once before any destructive work.
func migrateToolOnlyRecord(r record) record {
	r.Version = recordVersion
	r.Policy.Enabled = false
	r.Preparation = Preparation{State: stateNone}
	r.PreparationDetail = ""
	r.ApprovedRevision = 0
	r.Receipt = ""
	r.FirstMutation = false
	r.NextDueAt = nil
	if r.Operation != nil && r.Operation.State == stateRunning {
		finishOperation(&r, stateCancelled, "scope_migrated", time.Now())
	}
	return r
}

func validateRecord(r record) error {
	if r.Preparation.State != stateNone && r.Preparation.Choice != choiceBackup && r.Preparation.Choice != choiceSkip {
		return errors.New("invalid_preparation")
	}
	if r.Policy.Enabled && !approved(&r) {
		return errors.New("invalid_preparation")
	}
	if r.Operation == nil {
		return nil
	}
	if r.Operation.ID == "" {
		return errors.New("invalid_operation")
	}
	switch r.Operation.Kind {
	case kindAnalysis, kindCleanup, choiceBackup:
	default:
		return errors.New("invalid_operation")
	}
	switch r.Operation.State {
	case stateRunning, stateSucceeded, stateFailed, statePartial, stateCancelled:
	default:
		return errors.New("invalid_operation")
	}
	if r.Operation.State == stateRunning && r.Operation.Kind != choiceBackup && !r.Progress.Started {
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
	// Acquire the writer lock before reading either policy or task admission.
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
