package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// DeferredLaunchPrior is an opaque expected-prior-state token for
// SetTaskDeferredLaunchIfUnchanged, obtained from GetTaskDeferredLaunch or from
// AbsentDeferredLaunch.
//
// It is opaque, and it is produced by this package rather than supplied by the
// caller, for one reason: the comparison is over the whole stored deferred_launch
// value, and a caller holding a decoded map[string]interface{} cannot reconstruct
// those bytes reliably. Re-marshalling a decoded map turns every number into a
// float64, so a Unix-second ceiling_queued_at comes back as 1.7576064e+09 and the
// caller's compare-and-set can never win again. Deriving the token from the
// database's own bytes at read time removes that failure mode from the API.
//
// It is exported through GetTaskDeferredLaunch and SetTaskDeferredLaunchIfUnchanged
// as interface{} rather than as this named type, so a consumer package (the
// orchestrator's admission controller) can thread the token through a
// repository-shaped interface without importing this package — the same
// structural-interface boundary every other repoStore method already keeps.
type DeferredLaunchPrior struct {
	present   bool
	canonical string
}

// AbsentDeferredLaunch is the expected prior state meaning "the task carries no
// deferred_launch record". It is a legal prior, and it is what lets the first
// writer create the record: the create case — two concurrent first refusals each
// reading "no record" — is exactly the race this compare-and-set exists for, and
// the repository's stamp-based CAS primitives cannot express it at all.
func AbsentDeferredLaunch() interface{} { return DeferredLaunchPrior{} }

// GetTaskDeferredLaunch reads a task's deferred_launch record and the prior-state
// token that a subsequent SetTaskDeferredLaunchIfUnchanged compares against.
//
// The returned record decodes numbers as json.Number rather than float64, so a
// caller that reads, edits one key and writes back does not silently rewrite the
// others. A record whose stored value is not a JSON object decodes to a nil map
// with a present prior, leaving the caller free to replace it.
func (r *Repository) GetTaskDeferredLaunch(
	ctx context.Context, taskID string,
) (map[string]interface{}, interface{}, error) {
	var raw sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT metadata FROM tasks WHERE id = ?`), taskID).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, DeferredLaunchPrior{}, fmt.Errorf("task not found: %s", taskID)
		}
		return nil, DeferredLaunchPrior{}, err
	}
	metadata := "{}"
	if raw.Valid {
		metadata = raw.String
	}
	return decodeDeferredLaunch(metadata)
}

// SetTaskDeferredLaunchIfUnchanged writes the deferred_launch record only when the
// stored value still matches prior.
//
// The transaction, row lock and update query are the ones setMetadataKeyIfStamp
// already uses: one BeginTxx, then lockMetadataRow (SELECT … FOR UPDATE on
// Postgres, a transaction-scoped read on SQLite), then the json_set / jsonb_set
// single-key update, then commit. Only the comparison predicate is new.
//
// A lost comparison is reported through lostCompare with a nil error, and that
// distinction is the point: a lost compare is an ordinary, expected race whose
// handling is to re-read and re-apply, whereas an error is a failure that
// escalates to a refusal the system could not persist. Returning the first as the
// second would surface a red card for a case the design handles.
func (r *Repository) SetTaskDeferredLaunchIfUnchanged(
	ctx context.Context, taskID string, prior interface{}, value map[string]interface{},
) (stored bool, lostCompare bool, err error) {
	if value == nil {
		return false, false, fmt.Errorf("deferred launch value must not be nil for task %s", taskID)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return false, false, fmt.Errorf("failed to serialize deferred launch record: %w", err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer func() { _ = tx.Rollback() }()

	metadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return false, false, err
	}
	_, found, err := decodeDeferredLaunch(metadata)
	if err != nil {
		return false, false, err
	}
	if found != prior {
		// Commit rather than roll back: the read lock has nothing to undo, and
		// committing releases it immediately so the winner is not held up.
		if err := tx.Commit(); err != nil {
			return false, false, err
		}
		return false, true, nil
	}

	result, err := tx.ExecContext(ctx,
		r.db.Rebind(metadataKeyUpdateQuery("tasks", r.db.DriverName())),
		metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyDeferredLaunch, string(payload), r.nowUTC(), taskID)...)
	if err != nil {
		return false, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, false, err
	}
	if rows == 0 {
		return false, false, fmt.Errorf("task not found: %s", taskID)
	}
	if err := tx.Commit(); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// decodeDeferredLaunch splits a task's whole metadata JSON into the decoded
// deferred_launch record and its prior-state token. Both callers go through it so
// the read side and the locked compare side canonicalize identically.
func decodeDeferredLaunch(metadataJSON string) (map[string]interface{}, DeferredLaunchPrior, error) {
	trimmed := bytes.TrimSpace([]byte(metadataJSON))
	if len(trimmed) == 0 || string(trimmed) == jsonNull {
		return nil, DeferredLaunchPrior{}, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &metadata); err != nil {
		return nil, DeferredLaunchPrior{}, fmt.Errorf("failed to parse metadata: %w", err)
	}
	canonical, err := models.CanonicalJSON(metadata[models.MetaKeyDeferredLaunch])
	if err != nil {
		return nil, DeferredLaunchPrior{}, fmt.Errorf("failed to parse %s: %w", models.MetaKeyDeferredLaunch, err)
	}
	if canonical == nil {
		return nil, DeferredLaunchPrior{}, nil
	}

	prior := DeferredLaunchPrior{present: true, canonical: string(canonical)}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	var record map[string]interface{}
	if err := decoder.Decode(&record); err != nil {
		// Present but not an object. The caller decides what to do with it,
		// and the prior still describes exactly what is stored.
		return nil, prior, nil
	}
	return record, prior, nil
}

// TakeTaskDeferredLaunchWIPKeys removes the launch-intent keys a direct start
// claims, and only those, returning what it took so a failed start can put them
// back.
//
// The deferred_launch record carries independent meanings that share one key, so
// claiming the whole key would delete a meaning this caller does not own. The
// partition is by prefix rather than by an enumerated list, so neither writer has
// to track the other's keys: everything named by IsCeilingRecordKey stays, and
// everything else is the claim's.
//
// When removing the claimed keys empties the object the whole key is deleted,
// which is the entire behaviour for a record that carries no ceiling keys.
// A record holding only ceiling keys is not this caller's to claim, so the take
// is inert and reports nothing claimed.
func (r *Repository) TakeTaskDeferredLaunchWIPKeys(
	ctx context.Context, taskID string,
) (wip map[string]interface{}, claimed bool, err error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	metadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return nil, false, err
	}
	record, prior, err := decodeDeferredLaunch(metadata)
	if err != nil {
		return nil, false, err
	}
	if !prior.present {
		return nil, false, tx.Commit()
	}
	if record == nil {
		return nil, false, fmt.Errorf("%s on task %s is not a JSON object", models.MetaKeyDeferredLaunch, taskID)
	}

	claimedKeys := make(map[string]interface{})
	retained := make(map[string]interface{})
	for key, value := range record {
		if models.IsCeilingRecordKey(key) {
			retained[key] = value
			continue
		}
		claimedKeys[key] = value
	}
	if len(claimedKeys) == 0 {
		return nil, false, tx.Commit()
	}

	if len(retained) == 0 {
		if _, err := r.removeTaskMetadataKeyWithExecutor(ctx, tx, taskID, models.MetaKeyDeferredLaunch); err != nil {
			return nil, false, err
		}
		return claimedKeys, true, tx.Commit()
	}
	if err := r.writeDeferredLaunchLocked(ctx, tx, taskID, retained); err != nil {
		return nil, false, err
	}
	return claimedKeys, true, tx.Commit()
}

// RestoreTaskDeferredLaunchWIPKeys puts previously claimed keys back, merging
// into whatever the record holds now rather than replacing it.
//
// Restoring the pre-claim snapshot wholesale would overwrite anything written to
// the record while the launch was in flight. Merging is what lets the two
// meanings coexist: this caller puts back exactly the keys it took and leaves
// every other key at its current value.
func (r *Repository) RestoreTaskDeferredLaunchWIPKeys(
	ctx context.Context, taskID string, wip map[string]interface{},
) error {
	if len(wip) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	metadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return err
	}
	record, _, err := decodeDeferredLaunch(metadata)
	if err != nil {
		return err
	}
	merged := make(map[string]interface{}, len(record)+len(wip))
	maps.Copy(merged, record)
	maps.Copy(merged, wip)

	if err := r.writeDeferredLaunchLocked(ctx, tx, taskID, merged); err != nil {
		return err
	}
	return tx.Commit()
}

// writeDeferredLaunchLocked performs the single-key update inside a transaction
// that already holds the row.
func (r *Repository) writeDeferredLaunchLocked(
	ctx context.Context, tx *sqlx.Tx, taskID string, record map[string]interface{},
) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to serialize deferred launch record: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		r.db.Rebind(metadataKeyUpdateQuery("tasks", r.db.DriverName())),
		metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyDeferredLaunch, string(payload), r.nowUTC(), taskID)...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}
