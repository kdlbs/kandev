package service

import (
	"encoding/json"
	"strings"
)

// EncodeTaskLabels normalizes labels into the task store's canonical JSON
// array representation. It matches the HTTP task-create behavior: labels are
// trimmed, blank values are dropped, and duplicates retain first occurrence.
func EncodeTaskLabels(labels []string) string {
	return encodeTaskLabels(labels, false)
}

// EncodeTaskLabelsForUpdate encodes an explicitly supplied update value. Unlike
// create, an empty normalized result must be stored as [] to clear labels.
func EncodeTaskLabelsForUpdate(labels []string) string {
	return encodeTaskLabels(labels, true)
}

func encodeTaskLabels(labels []string, encodeEmpty bool) string {
	normalized := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	if len(normalized) == 0 && !encodeEmpty {
		return ""
	}
	encoded, _ := json.Marshal(normalized)
	return string(encoded)
}
