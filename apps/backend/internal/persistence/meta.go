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
	metaKeyLatestVersion                 = "latest_version"
	metaKeyLatestVersionURL              = "latest_version_url"
	metaKeyLatestVersionCheckedAt        = "latest_version_checked_at"
	metaKeyLatestNightlyVersion          = "latest_version_nightly"
	metaKeyLatestNightlyVersionURL       = "latest_version_nightly_url"
	metaKeyLatestNightlyVersionCheckedAt = "latest_version_nightly_checked_at"
)

type latestVersionKeys struct {
	version   string
	url       string
	checkedAt string
}

var stableLatestVersionKeys = latestVersionKeys{
	version:   metaKeyLatestVersion,
	url:       metaKeyLatestVersionURL,
	checkedAt: metaKeyLatestVersionCheckedAt,
}

var nightlyLatestVersionKeys = latestVersionKeys{
	version:   metaKeyLatestNightlyVersion,
	url:       metaKeyLatestNightlyVersionURL,
	checkedAt: metaKeyLatestNightlyVersionCheckedAt,
}

const metaTableDDL = `
CREATE TABLE IF NOT EXISTS kandev_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
)`

const metaKeyUpsert = `INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`

type metaKeyExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

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
	err := db.QueryRow(db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), key).Scan(&value)
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
	return writeMetaKey(db, db.Rebind(metaKeyUpsert), key, value)
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
// values use separate keys (latest_version, latest_version_url,
// latest_version_checked_at) on the existing key/value kandev_meta table, but
// the tuple is committed atomically.
func WriteLatestVersion(db *sqlx.DB, version, url string, checkedAt time.Time) error {
	return writeLatestVersion(db, stableLatestVersionKeys, version, url, checkedAt)
}

func writeLatestVersion(db *sqlx.DB, keys latestVersionKeys, version, url string, checkedAt time.Time) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("begin latest version write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := db.Rebind(metaKeyUpsert)
	if err := writeMetaKey(tx, query, keys.version, version); err != nil {
		return err
	}
	if err := writeMetaKey(tx, query, keys.url, url); err != nil {
		return err
	}
	ts := strconv.FormatInt(checkedAt.UTC().Unix(), 10)
	if err := writeMetaKey(tx, query, keys.checkedAt, ts); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit latest version write: %w", err)
	}
	return nil
}

// ReadLatestVersion returns the last-known latest release tag, its URL, and
// the timestamp of the last successful GitHub poll. Returns zero values when
// the keys are absent (e.g. on a fresh install before the first poll
// completes) and tolerates a subset of the three keys being missing.
func ReadLatestVersion(db *sqlx.DB) (string, string, time.Time, error) {
	return readLatestVersion(db, stableLatestVersionKeys)
}

func readLatestVersion(db *sqlx.DB, keys latestVersionKeys) (string, string, time.Time, error) {
	query := db.Rebind(`SELECT
		COALESCE(MAX(CASE WHEN key = ? THEN value END), ''),
		COALESCE(MAX(CASE WHEN key = ? THEN value END), ''),
		COALESCE(MAX(CASE WHEN key = ? THEN value END), '')
		FROM kandev_meta`)
	var version, url, tsRaw string
	if err := db.QueryRow(query, keys.version, keys.url, keys.checkedAt).Scan(
		&version,
		&url,
		&tsRaw,
	); err != nil {
		return "", "", time.Time{}, fmt.Errorf("read latest version tuple: %w", err)
	}
	var checkedAt time.Time
	if tsRaw != "" {
		secs, perr := strconv.ParseInt(tsRaw, 10, 64)
		if perr != nil {
			return "", "", time.Time{}, fmt.Errorf("parse %s %q: %w", keys.checkedAt, tsRaw, perr)
		}
		checkedAt = time.Unix(secs, 0).UTC()
	}
	return version, url, checkedAt, nil
}

func writeMetaKey(executor metaKeyExecutor, query, key, value string) error {
	if _, err := executor.Exec(query, key, value); err != nil {
		return fmt.Errorf("write meta key %q: %w", key, err)
	}
	return nil
}

// WriteLatestNightlyVersion persists the npm nightly target independently
// from the stable GitHub release cache.
func WriteLatestNightlyVersion(db *sqlx.DB, version, url string, checkedAt time.Time) error {
	return writeLatestVersion(db, nightlyLatestVersionKeys, version, url, checkedAt)
}

// ReadLatestNightlyVersion returns the isolated npm nightly target cache.
func ReadLatestNightlyVersion(db *sqlx.DB) (string, string, time.Time, error) {
	return readLatestVersion(db, nightlyLatestVersionKeys)
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
