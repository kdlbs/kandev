package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

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
type DeferredLaunchPrior struct {
	present   bool
	canonical string
}

// AbsentDeferredLaunch is the expected prior state meaning "the task carries no
// deferred_launch record". It is a legal prior, and it is what lets the first
// writer create the record: the create case — two concurrent first refusals each
// reading "no record" — is exactly the race this compare-and-set exists for, and
// the repository's stamp-based CAS primitives cannot express it at all.
func AbsentDeferredLaunch() DeferredLaunchPrior { return DeferredLaunchPrior{} }

// GetTaskDeferredLaunch reads a task's deferred_launch record and the prior-state
// token that a subsequent SetTaskDeferredLaunchIfUnchanged compares against.
//
// The returned record decodes numbers as json.Number rather than float64, so a
// caller that reads, edits one key and writes back does not silently rewrite the
// others. A record whose stored value is not a JSON object decodes to a nil map
// with a present prior, leaving the caller free to replace it.
func (r *Repository) GetTaskDeferredLaunch(
	ctx context.Context, taskID string,
) (map[string]interface{}, DeferredLaunchPrior, error) {
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
	ctx context.Context, taskID string, prior DeferredLaunchPrior, value map[string]interface{},
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
