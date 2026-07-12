// Package backups manages user-facing SQLite snapshots stored under
// <data-dir>/backups/. The package distinguishes two kinds of snapshots
// purely by filename prefix:
//
//   - "kandev-<...>.db"  -- AUTO snapshots written by the persistence layer
//     before pre-migration upgrades. Subject to the keep-newest-2 retention
//     in persistence.PruneBackups.
//
//   - "manual-<unix-ts>.db" -- MANUAL snapshots created by the user via the
//     System -> Backups UI. NEVER auto-pruned. Survive arbitrary calls to
//     persistence.PruneBackups because that function only matches the
//     "kandev-" prefix.
//
// retention.go documents the policy and exposes the auto/manual classifier
// used by List(). The actual pruning of auto snapshots lives in
// internal/persistence; this package never deletes auto files implicitly.
package backups

import "strings"

// Kind constants exposed in API responses.
const (
	KindAuto   = "auto"
	KindManual = "manual"
)

// autoPrefix and manualPrefix are the filename markers that distinguish
// the two snapshot families. ManualPrefix is also used when creating new
// manual snapshots so they avoid persistence.PruneBackups's "kandev-"
// glob.
const (
	autoPrefix   = "kandev-"
	manualPrefix = "manual-"
	dbSuffix     = ".db"
	// tmpSuffix marks an in-progress VACUUM INTO sidecar. classify() skips
	// it (no .db suffix) so it is never listed, and runCreate sweeps any
	// leftovers from a crashed run before writing a new snapshot.
	tmpSuffix = ".tmp"
)

// preResetPrefix marks recovery snapshots written by the Factory Reset
// flow. These must never be deleted via the regular Delete handler;
// they are the user's lifeline if a reset goes wrong.
const preResetPrefix = "kandev-pre-reset-"

// classify maps a filename to its snapshot Kind. Returns "" when the
// name is not a recognised snapshot (no .db suffix or unknown prefix).
func classify(name string) string {
	if !strings.HasSuffix(name, dbSuffix) {
		return ""
	}
	switch {
	case strings.HasPrefix(name, manualPrefix):
		return KindManual
	case strings.HasPrefix(name, autoPrefix):
		return KindAuto
	}
	return ""
}

// isPreResetSnapshot returns true if the file is a Factory Reset recovery
// snapshot and therefore not deletable via the regular Delete endpoint.
func isPreResetSnapshot(name string) bool {
	return strings.HasPrefix(name, preResetPrefix)
}
