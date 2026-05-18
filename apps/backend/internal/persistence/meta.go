package persistence

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

const metaTableDDL = `
CREATE TABLE IF NOT EXISTS kandev_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
)`

// ensureMetaTable creates the kandev_meta table if it does not exist.
func ensureMetaTable(db *sqlx.DB) error {
	if _, err := db.Exec(metaTableDDL); err != nil {
		return fmt.Errorf("create kandev_meta: %w", err)
	}
	return nil
}

// readKey returns the value for key, or "" when the key is absent.
func readKey(db *sqlx.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read meta key %q: %w", key, err)
	}
	return value, nil
}

// writeKey upserts key=value into kandev_meta.
func writeKey(db *sqlx.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("write meta key %q: %w", key, err)
	}
	return nil
}

// WriteVersion records currentVersion as the binary version that last
// successfully completed boot against this DB. Call this only after every
// repository initSchema has succeeded.
func WriteVersion(db *sqlx.DB, version string) error {
	if err := writeKey(db, "kandev_version", version); err != nil {
		return err
	}
	return nil
}

// hasUserTables returns true when the DB contains at least one table that is
// not part of the SQLite internal schema and not kandev_meta itself. This
// distinguishes a genuinely fresh DB from a pre-meta DB being upgraded.
func hasUserTables(db *sqlx.DB) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'kandev_meta'
	`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check user tables: %w", err)
	}
	return count > 0, nil
}

// shouldBackup returns true when a pre-migration backup should be taken.
//
//   - fresh DB with no user tables: no backup (clean first install)
//   - any tables exist but stored version is empty: pre-meta upgrade, back up
//   - stored version differs from current binary version: upgrade, back up
//   - stored version matches current: same release re-launched, no backup
func shouldBackup(stored, current string, userTables bool) bool {
	if stored == "" && !userTables {
		return false // fresh install
	}
	if stored != current {
		return true // upgrade (or pre-meta DB)
	}
	return false
}

// fallback returns s if non-empty, otherwise def.
func fallback(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
