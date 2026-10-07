package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func (r *UpdateUserSettingsRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateUserSettingsRequest
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"sidebar_fast_actions_enabled", "sidebar_new_task_style"} {
		if raw, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s cannot be null", key)
		}
	}
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = UpdateUserSettingsRequest(decoded)
	return nil
}
