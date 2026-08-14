// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"errors"
	"time"
)

// subagentContextNowRFC3339Nano returns the current UTC instant formatted at
// nanosecond precision — the shape every subagent-context kandev_meta
// activation key uses (AC-24 point 3), and the shape
// subagentPreciseTimestamp's normalization expects on the other side of a
// comparison.
func subagentContextNowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// subagentContextActivationKeyInsertSQL builds a write-once INSERT against
// kandev_meta for one activation key. ON CONFLICT DO NOTHING is what makes it
// write-once (AC-24, AC-24d, AC-24f): a key already present — including one
// written by a pre-amendment build at second precision (AC-24 point 5), or
// the other key of a pair repaired on a later boot (AC-24d revision 4) — is
// left exactly as it is, never rewritten. r.migrate.Apply takes no bind args,
// so key and value are inlined; both are always produced by this package,
// never caller/request input, so this is not a SQL-injection surface.
func subagentContextActivationKeyInsertSQL(key, value string) string {
	return "INSERT INTO kandev_meta (key, value) VALUES ('" + key + "', '" + value + "') ON CONFLICT (key) DO NOTHING"
}

// subagentContextActivationKeyPresent reports whether key already has a row
// in kandev_meta, via a single indexed point lookup on its primary key. This
// is a row-existence test, not a value-non-empty test (SR45): an empty-string
// backfill_through row still counts as present, since AC-24f defines the
// empty string as this key's legitimate sentinel value, not "absent".
func (r *Repository) subagentContextActivationKeyPresent(key string) (bool, error) {
	var value string
	err := r.db.QueryRow(r.db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), key).Scan(&value)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}
