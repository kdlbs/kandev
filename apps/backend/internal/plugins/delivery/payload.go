package delivery

import (
	"encoding/json"
	"fmt"
	"strings"
)

// messageSubjectPrefix marks the message.* subjects whose payloads carry the
// unstripped raw_content field for the native UI.
const messageSubjectPrefix = "message."

// rawContentKey holds a message's content including injected
// <kandev-system> blocks. Plugins only ever receive the stripped content.
const rawContentKey = "raw_content"

// sanitizePluginPayload removes fields plugins must never observe from a
// payload already copied by dataToMap, so other bus subscribers keep them.
func sanitizePluginPayload(subject string, payload map[string]any) {
	if strings.HasPrefix(subject, messageSubjectPrefix) {
		delete(payload, rawContentKey)
	}
}

// dataToMap converts a bus.Event's Data field (arbitrary JSON-serializable
// Go value: struct, map, nil, ...) into the map[string]any shape
// pluginsdk.Event.Payload expects, by round-tripping through JSON. A nil
// data value converts to a nil map (pluginsdk distinguishes "no payload"
// from "empty payload" across the wire).
func dataToMap(data interface{}) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal event data as map: %w", err)
	}
	return m, nil
}

// workspaceIDFromData extracts data["workspace_id"] when data is a
// map[string]interface{} carrying a string value, and returns "" otherwise
// (e.g. for struct-typed event data, or maps without the key).
func workspaceIDFromData(data interface{}) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := m["workspace_id"].(string)
	return id
}
