// Package maintenance implements the offline `kandev maintenance database`
// command: a CLI-invoked, exclusive-access retention and compaction pass
// over a stopped backend's SQLite database. It is deliberately independent
// of internal/system/database, which is a live, HTTP-triggered facade for
// the running backend's System > Database settings page - the two must
// never be conflated, since this package assumes exclusive access to the
// database file and internal/system/database assumes the opposite.
//
// Safety model (see docs/plans/pr-watch-and-storage-bounds/
// task-06-database-maintenance-command.md for the full contract):
//   - Dry-run (Analyze) is always read-only and is the default mode; it
//     never requires exclusive access.
//   - Destructive execution (Run with Execute=true) requires exclusive
//     access to the database (enforced via the same
//     internal/backendapp/ownershiplock mechanism the live backend uses at
//     boot, so a running backend and a maintenance run can never overlap)
//     and always takes a fresh, verified VACUUM INTO backup before deleting
//     anything.
//   - Optional staged compaction (Run with Compact=true) stages a second
//     VACUUM INTO copy, verifies it with PRAGMA integrity_check, and only
//     then atomically replaces the live file, retaining the pre-compaction
//     file as a documented rollback artifact.
//   - Retention only ever targets the three candidate categories exposed by
//     Task 05's read-only candidate queries (duplicate git snapshots,
//     obsolete plan revisions, orphaned message payloads); it never touches
//     task_session_messages, task_sessions, or any other conversational or
//     task-identity row.
package maintenance

import "errors"

// ErrUnsupportedDriver is returned when the configured database driver is
// not sqlite. Database maintenance is a SQLite-only capability; a
// PostgreSQL deployment manages retention/compaction through its own
// operator tooling.
var ErrUnsupportedDriver = errors.New("database maintenance requires the sqlite driver")

// ErrOwnershipConflict is returned when --execute is requested while
// another kandev process (the live backend, or a concurrent maintenance
// run) already owns the target database.
var ErrOwnershipConflict = errors.New("another kandev process owns this database; stop it before running --execute")

// ErrInsufficientDiskSpace is returned when the staging area does not have
// enough free space to hold a full compacted copy of the database
// alongside the live file.
var ErrInsufficientDiskSpace = errors.New("insufficient free disk space for a staged compaction copy")

// ErrIntegrityCheckFailed is returned when PRAGMA integrity_check reports a
// problem with a freshly staged (backup or compaction) snapshot. The
// original database is left untouched whenever this is returned.
var ErrIntegrityCheckFailed = errors.New("staged database failed PRAGMA integrity_check")

// ErrBackupUnverified is returned when the pre-execution backup could not
// be created or verified. Execution never proceeds past this point.
var ErrBackupUnverified = errors.New("could not create or verify a pre-execution backup")
