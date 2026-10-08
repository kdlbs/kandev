package store

import (
	"encoding/json"
	"fmt"
)

func (r *sqliteRepository) migrateTurnChangedFilesPreference() error {
	return r.migrateUserSettingsPreference("turn changed-files preference", backfillTurnChangedFilesPreference)
}

func (r *sqliteRepository) migrateUserSettingsPreference(name string, backfill func(string) ([]byte, bool, error)) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var rows []struct {
		ID       string `db:"id"`
		Settings string `db:"settings"`
		Revision int64  `db:"settings_revision"`
	}
	if err := tx.Select(&rows, "SELECT id, settings, settings_revision FROM users"); err != nil {
		return err
	}
	for _, row := range rows {
		updated, changed, err := backfill(row.Settings)
		if err != nil {
			return fmt.Errorf("%s migration for %s: %w", name, row.ID, err)
		}
		if !changed {
			continue
		}
		result, err := tx.Exec(tx.Rebind("UPDATE users SET settings = ?, settings_revision = settings_revision + 1 WHERE id = ? AND settings_revision = ?"), string(updated), row.ID, row.Revision)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("concurrent %s migration", name)
		}
	}
	return tx.Commit()
}

func backfillTurnChangedFilesPreference(raw string) ([]byte, bool, error) {
	if raw == "" {
		raw = "{}"
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, false, err
	}
	if fields == nil {
		return nil, false, fmt.Errorf("settings must be an object")
	}
	if _, exists := fields["show_turn_changed_files"]; exists {
		return nil, false, nil
	}
	fields["show_turn_changed_files"] = json.RawMessage("true")
	encoded, err := json.Marshal(fields)
	return encoded, true, err
}
