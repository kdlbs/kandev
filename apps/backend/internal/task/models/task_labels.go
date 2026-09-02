package models

import (
	"encoding/json"
	"strings"
)

// DecodeTaskLabels converts the task store's JSON-array label representation
// into the plugin-facing slice. Legacy or malformed stored data falls back to
// an empty result so a read cannot fail or expose an invalid shape to callers.
func DecodeTaskLabels(encoded string) []string {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || encoded == "[]" {
		return nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(encoded), &labels); err != nil {
		return nil
	}
	return labels
}
