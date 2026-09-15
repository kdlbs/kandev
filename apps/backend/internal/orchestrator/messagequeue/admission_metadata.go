package messagequeue

import (
	"encoding/json"
	"errors"
)

// ErrInvalidQueueAdmissionIDs reports malformed internal provenance metadata.
// A merge must leave both queue entries untouched when its provenance cannot
// be normalized safely.
var ErrInvalidQueueAdmissionIDs = errors.New("invalid queue admission IDs")

func unionQueueAdmissionIDs(target, source map[string]interface{}) ([]string, bool) {
	seen := make(map[string]struct{})
	var union []string
	for _, metadata := range []map[string]interface{}{target, source} {
		ids, ok := normalizeQueueAdmissionIDs(metadata[MetadataQueueAdmissionIDs])
		if !ok {
			return nil, false
		}
		for _, id := range ids {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			union = append(union, id)
		}
	}
	return union, true
}

func normalizeQueueAdmissionIDs(raw interface{}) ([]string, bool) {
	if raw == nil {
		return nil, true
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var ids []string
	if err := json.Unmarshal(encoded, &ids); err != nil {
		return nil, false
	}
	for _, id := range ids {
		if id == "" {
			return nil, false
		}
	}
	return ids, true
}
