package store

import (
	"encoding/json"
	"fmt"
)

const newSidebarPresentationJSON = `{"sidebar_fast_actions_enabled":false,"sidebar_new_task_style":"simple","show_turn_changed_files":true}`
const sidebarNewTaskStyleCompact = "compact"

// Existing accounts receive legacy defaults; insert paths explicitly seed new defaults.
func (r *sqliteRepository) migrateSidebarPresentation() error {
	return r.migrateUserSettingsPreference("sidebar preference", backfillSidebarPresentation)
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
