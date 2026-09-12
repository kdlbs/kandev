package models

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Omitted presentation fields have compatibility defaults. Explicit nulls
// and empty layouts are not valid new settings values.
func decodeThreadViewJSON(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	var fields struct {
		Layout           json.RawMessage `json:"layout"`
		AutoHideComposer json.RawMessage `json:"auto_hide_composer"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	layout := string(bytes.TrimSpace(fields.Layout))
	if layout == "null" || layout == `""` {
		return errors.New("thread layout must be columns or grid")
	}
	if bytes.Equal(bytes.TrimSpace(fields.AutoHideComposer), []byte("null")) {
		return errors.New("auto_hide_composer must be a boolean")
	}
	return nil
}

func (v *ThreadView) UnmarshalJSON(data []byte) error {
	type value ThreadView
	var decoded value
	if err := decodeThreadViewJSON(data, &decoded); err != nil {
		return err
	}
	*v = ThreadView(decoded)
	return nil
}

func (v *ThreadViewDraft) UnmarshalJSON(data []byte) error {
	type value ThreadViewDraft
	var decoded value
	if err := decodeThreadViewJSON(data, &decoded); err != nil {
		return err
	}
	*v = ThreadViewDraft(decoded)
	return nil
}
