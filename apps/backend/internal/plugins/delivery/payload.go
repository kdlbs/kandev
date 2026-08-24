package delivery

import (
	"encoding/json"
	"fmt"
)

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

// workspaceIDFromPayload extracts the workspace after dataToMap has applied
// the event's JSON tags. Reading the raw bus value only worked for map events;
// most domain events, including automation.triggered, are structs and would
// silently lose their server-stamped workspace provenance at the plugin wire.
func workspaceIDFromPayload(payload map[string]any) string {
	id, _ := payload["workspace_id"].(string)
	return id
}
