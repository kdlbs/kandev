package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	metaKeyLatestVersion          = "latest_version"
	metaKeyLatestVersionURL       = "latest_version_url"
	metaKeyLatestVersionCheckedAt = "latest_version_checked_at"
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

// WriteLatestVersion persists the highest semver tag from the GitHub Releases
// poll, its release URL, and the timestamp of the successful poll. The three
// values are written under separate keys (latest_version,
// latest_version_url, latest_version_checked_at) on the existing key/value
// kandev_meta table.
func WriteLatestVersion(db *sqlx.DB, version, url string, checkedAt time.Time) error {
	if err := writeKey(db, metaKeyLatestVersion, version); err != nil {
		return err
	}
	if err := writeKey(db, metaKeyLatestVersionURL, url); err != nil {
		return err
	}
	ts := strconv.FormatInt(checkedAt.UTC().Unix(), 10)
	return writeKey(db, metaKeyLatestVersionCheckedAt, ts)
}

// ReadLatestVersion returns the last-known latest release tag, its URL, and
// the timestamp of the last successful GitHub poll. Returns zero values when
// the keys are absent (e.g. on a fresh install before the first poll
// completes) and tolerates a subset of the three keys being missing.
func ReadLatestVersion(db *sqlx.DB) (string, string, time.Time, error) {
	version, err := readKey(db, metaKeyLatestVersion)
	if err != nil {
		return "", "", time.Time{}, err
	}
	url, err := readKey(db, metaKeyLatestVersionURL)
	if err != nil {
		return "", "", time.Time{}, err
	}
	tsRaw, err := readKey(db, metaKeyLatestVersionCheckedAt)
	if err != nil {
		return "", "", time.Time{}, err
	}
	var checkedAt time.Time
	if tsRaw != "" {
		secs, perr := strconv.ParseInt(tsRaw, 10, 64)
		if perr != nil {
			return "", "", time.Time{}, fmt.Errorf("parse latest_version_checked_at %q: %w", tsRaw, perr)
		}
		checkedAt = time.Unix(secs, 0).UTC()
	}
	return version, url, checkedAt, nil
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
