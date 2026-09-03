package store

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/kandev/kandev/internal/user/models"
)

// decodeSidebarTaskColors normalizes each stored entry independently so one
// malformed task decision cannot hide valid decisions or other settings.
func decodeSidebarTaskColors(raw json.RawMessage) map[string]*string {
	colors := map[string]*string{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return colors
	}

	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &encoded); err != nil || encoded == nil {
		return colors
	}
	keys := make([]string, 0, len(encoded))
	for taskID := range encoded {
		keys = append(keys, taskID)
	}
	sort.Strings(keys)
	for _, taskID := range keys {
		if len(colors) >= models.MaxSidebarTaskColors {
			break
		}
		if !models.IsValidSidebarTaskColorTaskID(taskID) {
			continue
		}
		color, valid := decodeSidebarTaskColorValue(encoded[taskID])
		if !valid {
			continue
		}
		colors[taskID] = color
	}
	trimSidebarTaskColors(colors, keys)
	return colors
}

func decodeSidebarTaskColorValue(raw json.RawMessage) (*string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	var color string
	if err := json.Unmarshal(trimmed, &color); err != nil || !models.IsValidSidebarTaskColor(color) {
		return nil, false
	}
	return &color, true
}

// trimSidebarTaskColors keeps normalized storage within the encoded-size cap.
// Sorted removal makes the fallback deterministic for hand-edited blobs.
func trimSidebarTaskColors(colors map[string]*string, keys []string) {
	encoded, err := json.Marshal(colors)
	if err != nil {
		return
	}
	for len(encoded) > models.MaxSidebarTaskColorsJSONBytes && len(keys) > 0 {
		last := keys[len(keys)-1]
		delete(colors, last)
		keys = keys[:len(keys)-1]
		encoded, err = json.Marshal(colors)
		if err != nil {
			return
		}
	}
}
