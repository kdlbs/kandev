package statussummary

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func isTaskErrorRefreshEvent(eventType string) bool {
	return eventType == events.TaskUpdated || eventType == events.TaskStateChanged
}

func isPendingSensitiveEvent(eventType string, data map[string]interface{}) bool {
	switch eventType {
	case events.TaskUpdated,
		events.TaskSessionStateChanged,
		events.MessageAdded,
		events.ClarificationAnswered,
		events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled,
		events.ClarificationStaleDismissed,
		events.PermissionRequestReceived:
		return true
	case events.MessageUpdated, events.MessageDeleted:
		messageType := strings.ToLower(stringField(data, "type"))
		return pendingActionForMessage(messageType, boolValue(data["requests_input"])) != ""
	default:
		return false
	}
}

func (p *Projector) refreshPendingLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
) (bool, error) {
	actions, err := p.loadPendingActions(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("load pending actions for task status summary %q: %w", taskID, err)
	}
	next := make(map[string]string, len(actions))
	for sessionID, action := range actions {
		sessionID = strings.TrimSpace(sessionID)
		action = strings.TrimSpace(action)
		if sessionID == "" || (action != pendingClarification && action != pendingPermission) {
			continue
		}
		next[sessionID] = action
	}
	changed := !pendingActionsEqual(state.pending, next)
	previousTaskPending := state.taskPending
	state.pendingObserved = true
	state.pending = next
	state.pendingRequests = make(map[string]pendingRequestIdentity)
	recomputeTaskPending(state)
	return changed || state.taskPending != previousTaskPending, nil
}

func pendingActionsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for sessionID, action := range left {
		if right[sessionID] != action {
			return false
		}
	}
	return true
}

func (p *Projector) clearPendingLocked(state *projectionState, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	state.pendingObserved = true
	_, existed := state.pending[sessionID]
	delete(state.pending, sessionID)
	delete(state.pendingRequests, sessionID)
	// A restarted projector has only the aggregate taskPending baseline, not
	// the original per-session entry. Recompute before deciding this is a no-op
	// so stale-dismissed events still clear and persist that restored value.
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
	existing, ok := state.errors[sessionID]
	if !ok {
		return false
	}
	// Remember which error was cleared. Session events carry a `session_metadata`
	// snapshot taken when the publisher read the session, which can predate the
	// clear — without this, such an event puts the very same error straight back.
	// A genuinely new failure has a different stamp and is still applied.
	if existing != nil && existing.Stamp != "" {
		state.clearedErrorStamps[sessionID] = existing.Stamp
	}
	delete(state.errors, sessionID)
	return true
}

// errorFromMetadata reads the session's stored error record. `observed` is true
// whenever the `last_agent_error` key is present at all: a dismissed record, or
// the JSON null the orchestrator writes when a turn completes, is an
// authoritative "no active error" for that session rather than an absence of
// information, so the caller must clear rather than ignore it.
//
// The key must be tested for presence before its type: a JSON null arrives as a
// nil value, and asserting `.(map[string]interface{})` on that is
// indistinguishable from the key being missing entirely. Reading it as missing
// would leave a restored error armed forever on any session whose record was
// cleared.
func errorFromMetadata(
	now time.Time,
	sessionID string,
	metadata map[string]interface{},
) (summary *ActiveErrorSummary, observed bool) {
	value, present := metadata["last_agent_error"]
	if !present {
		return nil, false
	}
	raw, ok := value.(map[string]interface{})
	if !ok {
		// Null or malformed: the key exists, so the session has no active error.
		return nil, true
	}
	active, ok := errorFromMap(now, sessionID, raw)
	if !ok {
		return nil, true
	}
	return active, true
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
		SessionID:        truncateString(sessionID, maxSessionIDBytes),
		TaskRepositoryID: truncateString(stringField(data, "task_repository_id"), maxTaskRepositoryIDBytes),
		Stamp:            truncateString(stamp, maxActiveErrorStampBytes),
		OccurredAt:       occurredAt.UTC(),
		Preview:          preview,
		Category:         truncateString(firstString(data, "category", "code"), maxActiveErrorCategoryBytes),
		RecoveryActions:  normalizeRecoveryActions(recoveryActionsValue(data["recovery_actions"])),
	}, true
}

func errorEqual(a, b *ActiveErrorSummary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.SessionID == b.SessionID && a.TaskRepositoryID == b.TaskRepositoryID &&
		a.Stamp == b.Stamp && a.OccurredAt.Equal(b.OccurredAt) &&
		a.Preview == b.Preview && a.Category == b.Category &&
		slices.Equal(a.RecoveryActions, b.RecoveryActions)
}

func normalizeRecoveryActions(actions []string) []string {
	return models.NormalizeRecoveryActions(actions)
}

func recoveryActionsValue(value interface{}) []string {
	switch actions := value.(type) {
	case []string:
		return append([]string(nil), actions...)
	case []interface{}:
		out := make([]string, 0, len(actions))
		for _, action := range actions {
			if value, ok := action.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
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
	case *time.Time:
		if value != nil {
			return *value
		}
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func maxTimePtr(left, right *time.Time) *time.Time {
	left = cloneTimePtr(left)
	right = cloneTimePtr(right)
	if left == nil {
		return right
	}
	if right == nil || !right.After(*left) {
		return left
	}
	return right
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
