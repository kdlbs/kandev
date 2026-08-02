package statussummary

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"
)

func (p *Projector) clearPendingLocked(state *projectionState, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	state.pendingObserved = true
	_, existed := state.pending[sessionID]
	if existed {
		delete(state.pending, sessionID)
	}
	previousTaskPending := state.taskPending
	recomputeTaskPending(state)
	return existed || previousTaskPending != state.taskPending
}

func recomputeTaskPending(state *projectionState) {
	state.taskPending = ""
	for _, action := range state.pending {
		if action == pendingPermission {
			state.taskPending = action
			return
		}
		if action == pendingClarification {
			state.taskPending = action
		}
	}
}

func (p *Projector) clearErrorLocked(state *projectionState, sessionID string) bool {
	state.errorsObserved = true
	if _, ok := state.errors[sessionID]; !ok {
		return false
	}
	delete(state.errors, sessionID)
	return true
}

func errorFromMetadata(now time.Time, sessionID string, metadata map[string]interface{}) (*ActiveErrorSummary, bool) {
	raw, ok := metadata["last_agent_error"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return errorFromMap(now, sessionID, raw)
}

func errorFromMap(now time.Time, sessionID string, data map[string]interface{}) (*ActiveErrorSummary, bool) {
	message := firstString(data, "preview", "message", "error_message")
	if message == "" {
		return nil, false
	}
	if dismissed := stringField(data, "dismissed_at"); dismissed != "" || boolValue(data["dismissed"]) {
		return nil, false
	}
	occurredAt := timeValue(data["occurred_at"])
	if occurredAt.IsZero() {
		occurredAt = timeValue(data["updated_at"])
	}
	if occurredAt.IsZero() {
		occurredAt = now.UTC()
	}
	preview := truncateString(message, MaxActiveErrorPreviewBytes)
	stamp := stringField(data, "stamp")
	if stamp == "" {
		stamp = occurredAt.UTC().Format(time.RFC3339Nano) + ":" + preview
	}
	return &ActiveErrorSummary{
		SessionID:  truncateString(sessionID, maxSessionIDBytes),
		Stamp:      truncateString(stamp, maxActiveErrorStampBytes),
		OccurredAt: occurredAt.UTC(),
		Preview:    preview,
	}, true
}

func errorEqual(a, b *ActiveErrorSummary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.SessionID == b.SessionID && a.Stamp == b.Stamp &&
		a.OccurredAt.Equal(b.OccurredAt) && a.Preview == b.Preview
}

func eventDataMap(data interface{}) (map[string]interface{}, error) {
	if data == nil {
		return nil, nil
	}
	if mapped, ok := data.(map[string]interface{}); ok {
		return mapped, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("decode status source payload: %w", err)
	}
	var mapped map[string]interface{}
	if err := json.Unmarshal(encoded, &mapped); err != nil {
		return nil, fmt.Errorf("decode status source payload: %w", err)
	}
	return mapped, nil
}

func stringField(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func firstString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringField(data, key); value != "" {
			return value
		}
	}
	return ""
}

func stringFromNullable(value interface{}) string {
	if value == nil {
		return ""
	}
	out, _ := value.(string)
	return out
}

func boolValue(value interface{}) bool {
	out, _ := value.(bool)
	return out
}

func intValue(value interface{}) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), true
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	case string:
		parsed, err := strconv.Atoi(number)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func intValueOrZero(value interface{}) int {
	parsed, _ := intValue(value)
	return parsed
}

func nonNegativeInt(data map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if value, ok := intValue(data[key]); ok {
			return maxInt(value, 0)
		}
	}
	return 0
}

func changedFileCount(data map[string]interface{}) int {
	if value, ok := intValue(data["changed_files"]); ok {
		return maxInt(value, 0)
	}
	count := 0
	for _, key := range []string{"modified", "added", "deleted", "untracked", "renamed"} {
		if values, ok := data[key].([]interface{}); ok {
			count += len(values)
		}
	}
	return count
}

func timeValue(value interface{}) time.Time {
	switch value := value.(type) {
	case time.Time:
		return value
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func equalGitSummary(a, b GitSummary) bool { return a == b }

func cloneSummary(summary *TaskStatusSummary) *TaskStatusSummary {
	if summary == nil {
		return nil
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		copy := *summary
		return &copy
	}
	var clone TaskStatusSummary
	if err := json.Unmarshal(encoded, &clone); err != nil {
		copy := *summary
		return &copy
	}
	return &clone
}

func truncateString(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
