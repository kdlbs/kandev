package store

import (
	"encoding/json"
	"fmt"
)

const newSidebarPresentationJSON = `{"sidebar_fast_actions_enabled":false,"sidebar_new_task_style":"simple"}`
const sidebarNewTaskStyleCompact = "compact"

// Existing accounts receive legacy defaults; insert paths explicitly seed new defaults.
func (r *sqliteRepository) migrateSidebarPresentation() error {
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
	if err = tx.Select(&rows, "SELECT id, settings, settings_revision FROM users"); err != nil {
		return err
	}
	for _, row := range rows {
		raw, changed, err := backfillSidebarPresentation(row.Settings)
		if err != nil {
			return fmt.Errorf("sidebar preference migration for %s: %w", row.ID, err)
		}
		if !changed {
			continue
		}
		result, err := tx.Exec(tx.Rebind("UPDATE users SET settings = ?, settings_revision = settings_revision + 1 WHERE id = ? AND settings_revision = ?"), string(raw), row.ID, row.Revision)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("concurrent sidebar preference migration")
		}
	}
	return tx.Commit()
}

func backfillSidebarPresentation(raw string) ([]byte, bool, error) {
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
	changed := false
	if _, ok := fields["sidebar_fast_actions_enabled"]; !ok {
		fields["sidebar_fast_actions_enabled"] = json.RawMessage("true")
		changed = true
	}
	if _, ok := fields["sidebar_new_task_style"]; !ok {
		fields["sidebar_new_task_style"] = json.RawMessage(`"` + sidebarNewTaskStyleCompact + `"`)
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	encoded, err := json.Marshal(fields)
	return encoded, changed, err
}
