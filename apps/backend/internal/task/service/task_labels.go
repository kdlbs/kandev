package service

import (
	"encoding/json"
	"fmt"
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

// ValidateTaskLabels checks that a stored labels value is a valid JSON array
// containing only non-blank string elements. It rejects null, scalars, and
// objects — json.Unmarshal of null into []string succeeds silently (nil
// slice), so non-array semantics are caught explicitly. This is a defense-
// in-depth guard on write paths where raw label strings bypass
// EncodeTaskLabels normalization.
func ValidateTaskLabels(encoded string) error {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return fmt.Errorf("labels must be a JSON array, got %q", encoded)
	}
	if trimmed == "[]" {
		return nil
	}
	if trimmed == "null" || !strings.HasPrefix(trimmed, "[") {
		return fmt.Errorf("labels must be a JSON array, got %q", encoded)
	}
	var labels []string
	if err := json.Unmarshal([]byte(encoded), &labels); err != nil {
		return fmt.Errorf("invalid labels JSON: %w", err)
	}
	for i, v := range labels {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("label[%d] must not be blank", i)
		}
	}
	return nil
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
